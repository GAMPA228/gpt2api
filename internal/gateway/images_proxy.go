package gateway

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/432539/gpt2api/internal/image"
	"github.com/432539/gpt2api/internal/middleware"
	"github.com/432539/gpt2api/internal/upstream/chatgpt"
	"github.com/432539/gpt2api/pkg/logger"
)

// imageUpscaleCache caches upscaled outputs (2k/4k).
var imageUpscaleCache = image.NewUpscaleCache(0, 0)

// imageRawCache caches original bytes to reduce repeated upstream fetches.
var imageRawCache = image.NewUpscaleCache(0, 0)

// ImageAccountResolver resolves account secrets/proxy used by image proxy fetches.
type ImageAccountResolver interface {
	AuthToken(ctx context.Context, accountID uint64) (at, deviceID, cookies string, err error)
	ProxyURL(ctx context.Context, accountID uint64) string
}

// ImageProxy serves signed public proxy URL:
// GET /p/img/:task_id/:idx?exp=...&sig=...
func (h *ImagesHandler) ImageProxy(c *gin.Context) {
	taskID := c.Param("task_id")
	idxStr := c.Param("idx")
	expStr := c.Query("exp")
	sig := c.Query("sig")
	if taskID == "" || idxStr == "" || expStr == "" || sig == "" {
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}

	idx, err := strconv.Atoi(idxStr)
	if err != nil || idx < 0 || idx > 64 {
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}
	expMs, err := strconv.ParseInt(expStr, 10, 64)
	if err != nil {
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}
	if !image.VerifyImageProxySig(taskID, idx, expMs, sig) {
		c.AbortWithStatus(http.StatusForbidden)
		return
	}
	if h.DAO == nil {
		c.AbortWithStatus(http.StatusServiceUnavailable)
		return
	}

	t, err := h.DAO.Get(c.Request.Context(), taskID)
	if err != nil {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	h.serveTaskImage(c, t, idx)
}

// MyImageProxy serves stable JWT-protected URL for "my image history":
// GET /api/me/images/proxy/:task_id/:idx
func (h *ImagesHandler) MyImageProxy(c *gin.Context) {
	uid := middleware.UserID(c)
	if uid == 0 {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	taskID := c.Param("task_id")
	idxStr := c.Param("idx")
	if taskID == "" || idxStr == "" {
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}
	idx, err := strconv.Atoi(idxStr)
	if err != nil || idx < 0 || idx > 64 {
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}
	if h.DAO == nil {
		c.AbortWithStatus(http.StatusServiceUnavailable)
		return
	}
	t, err := h.DAO.Get(c.Request.Context(), taskID)
	if err != nil || t.UserID != uid {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	h.serveTaskImage(c, t, idx)
}

func (h *ImagesHandler) serveTaskImage(c *gin.Context, t *image.Task, idx int) {
	fids := t.DecodeFileIDs()
	if idx >= len(fids) {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	if t.AccountID == 0 || h.ImageAccResolver == nil {
		c.AbortWithStatus(http.StatusServiceUnavailable)
		return
	}

	scale := image.ValidateUpscale(t.Upscale)
	rawKey := fmt.Sprintf("%s|%d|raw", t.TaskID, idx)
	scaleKey := ""
	if scale != "" {
		scaleKey = fmt.Sprintf("%s|%d|%s", t.TaskID, idx, scale)
		if data, ctCache, ok := imageUpscaleCache.Get(scaleKey); ok {
			c.Header("Cache-Control", "private, max-age=3600")
			c.Header("X-Upscale", scale+";cache=hit")
			c.Data(http.StatusOK, ctCache, data)
			return
		}
	}
	if scale == "" {
		if data, ctCache, ok := imageRawCache.Get(rawKey); ok {
			c.Header("Cache-Control", "private, max-age=1800")
			c.Header("X-Image-Cache", "raw-hit")
			c.Data(http.StatusOK, ctCache, data)
			return
		}
	}
	if scale != "" {
		if raw, rawCT, ok := imageRawCache.Get(rawKey); ok {
			imageUpscaleCache.Acquire()
			upBytes, upCT, err := image.DoUpscale(raw, scale)
			imageUpscaleCache.Release()
			if err == nil && len(upBytes) > 0 {
				if upCT == "" {
					upCT = "image/png"
				}
				imageUpscaleCache.Put(scaleKey, upBytes, upCT)
				c.Header("Cache-Control", "private, max-age=3600")
				c.Header("X-Upscale", scale+";cache=miss")
				c.Header("X-Image-Cache", "raw-hit")
				c.Data(http.StatusOK, upCT, upBytes)
				return
			}
			if rawCT == "" {
				rawCT = "image/png"
			}
			c.Header("Cache-Control", "private, max-age=1800")
			c.Header("X-Upscale", scale+";err")
			c.Header("X-Image-Cache", "raw-hit")
			c.Data(http.StatusOK, rawCT, raw)
			return
		}
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 60*time.Second)
	defer cancel()

	at, deviceID, cookies, err := h.ImageAccResolver.AuthToken(ctx, t.AccountID)
	if err != nil {
		logger.L().Warn("image proxy resolve account",
			zap.Error(err), zap.Uint64("account_id", t.AccountID))
		c.AbortWithStatus(http.StatusBadGateway)
		return
	}
	proxyURL := h.ImageAccResolver.ProxyURL(ctx, t.AccountID)

	cli, err := chatgpt.New(chatgpt.Options{
		AuthToken: at,
		DeviceID:  deviceID,
		ProxyURL:  proxyURL,
		Cookies:   cookies,
		Timeout:   h.upstreamTimeout(),
	})
	if err != nil {
		logger.L().Warn("image proxy build client", zap.Error(err))
		c.AbortWithStatus(http.StatusBadGateway)
		return
	}

	ref := fids[idx] // "sed:xxx" or plain id
	signedURL, err := cli.ImageDownloadURL(ctx, t.ConversationID, ref)
	if err != nil {
		logger.L().Warn("image proxy download_url",
			zap.Error(err), zap.String("task_id", t.TaskID), zap.String("ref", ref))
		c.AbortWithStatus(http.StatusBadGateway)
		return
	}

	body, ct, err := cli.FetchImage(ctx, signedURL, 16*1024*1024)
	if err != nil {
		logger.L().Warn("image proxy fetch",
			zap.Error(err), zap.String("task_id", t.TaskID))
		c.AbortWithStatus(http.StatusBadGateway)
		return
	}
	if ct == "" {
		ct = "image/png"
	}
	imageRawCache.Put(rawKey, body, ct)

	if scale != "" {
		imageUpscaleCache.Acquire()
		upBytes, upCT, err := image.DoUpscale(body, scale)
		imageUpscaleCache.Release()
		if err != nil {
			logger.L().Warn("image proxy upscale",
				zap.Error(err), zap.String("task_id", t.TaskID), zap.String("scale", scale))
			c.Header("Cache-Control", "private, max-age=1800")
			c.Header("X-Upscale", scale+";err")
			c.Header("X-Image-Cache", "raw-miss")
			c.Data(http.StatusOK, ct, body)
			return
		}
		if upCT != "" {
			ct = upCT
		}
		if len(upBytes) > 0 {
			body = upBytes
			imageUpscaleCache.Put(scaleKey, body, ct)
			c.Header("X-Upscale", scale+";cache=miss")
		} else {
			c.Header("X-Upscale", scale+";noop")
		}
		c.Header("Cache-Control", "private, max-age=3600")
		c.Header("X-Image-Cache", "raw-miss")
		c.Data(http.StatusOK, ct, body)
		return
	}

	c.Header("Cache-Control", "private, max-age=1800")
	c.Header("X-Image-Cache", "raw-miss")
	c.Data(http.StatusOK, ct, body)
}

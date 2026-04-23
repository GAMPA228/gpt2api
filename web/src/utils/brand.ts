export interface BrandParts {
  brand: string
  repo: string
  repoLabel: string
  sep: string
}

export function brandParts(): BrandParts {
  return {
    brand: 'GPT2API',
    repo: 'github.com/GAMPA228/gpt2api',
    repoLabel: '开源地址 ',
    sep: '·',
  }
}

export function brandPlainText(): string {
  const p = brandParts()
  return `${p.brand} ${p.sep} ${p.repoLabel}${p.repo}`
}

let warned = false
export function printBrandToConsole(): void {
  if (warned) return
  warned = true
  try {
    const p = brandParts()
    // eslint-disable-next-line no-console
    console.log(
      `%c${p.brand}%c  ${p.sep}  ${p.repoLabel}https://${p.repo}`,
      'font-weight:700;color:#409eff;font-size:13px;',
      'color:#909399;font-size:12px;',
    )
  } catch {
    /* ignore */
  }
}

let guardStarted = false
export function startBrandGuard(): void {
  if (guardStarted || typeof window === 'undefined') return
  guardStarted = true
  const check = () => {
    try {
      const p = brandParts()
      const html = document.body?.innerText || ''
      if (html.indexOf(p.repo) < 0) ensureShadowFooter()
    } catch {
      /* ignore */
    }
  }
  setTimeout(check, 2000)
  setInterval(check, 30000)
}

function ensureShadowFooter(): void {
  const id = '__gpt2api_brand_guard__'
  if (document.getElementById(id)) return
  const p = brandParts()
  const el = document.createElement('div')
  el.id = id
  el.style.cssText = [
    'position:fixed',
    'left:0',
    'right:0',
    'bottom:0',
    'z-index:2147483646',
    'padding:6px 16px',
    'font-size:12px',
    'line-height:1.6',
    'color:#909399',
    'background:rgba(255,255,255,0.92)',
    'backdrop-filter:blur(6px)',
    'border-top:1px solid rgba(0,0,0,0.06)',
    'text-align:center',
    'pointer-events:auto',
  ].join(';')
  el.innerHTML = [
    `<b style="color:#409eff">${escapeHtml(p.brand)}</b>`,
    `${escapeHtml(p.repoLabel)}<a href="https://${escapeHtml(p.repo)}" target="_blank" rel="noopener" style="color:#409eff;text-decoration:none">${escapeHtml(p.repo)}</a>`,
  ].join(` ${escapeHtml(p.sep)} `)
  document.body.appendChild(el)
}

function escapeHtml(s: string): string {
  return s
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
}

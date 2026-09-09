config => {
  if (location.origin !== 'https://www.instagram.com') {
    return Promise.reject(new Error('Instagram CAPTCHA requires the Instagram origin'))
  }
  const previous = window.__mautrixInstagramCaptcha
  if (previous?.instance === config.instance) return previous.promise
  previous?.cleanup()

  const overlay = document.createElement('div')
  overlay.style.cssText = 'position:fixed;inset:0;z-index:1999999999;background:white;' +
    'display:flex;flex-direction:column;align-items:center;justify-content:center;' +
    'gap:20px;padding:16px;overflow:auto;color:#111;font:16px sans-serif'
  const title = document.createElement('strong')
  title.textContent = 'Complete Instagram verification'
  const notice = document.createElement('p')
  notice.textContent = 'Solve the CAPTCHA below to continue your existing login.'
  const frame = document.createElement('iframe')
  frame.title = 'Instagram CAPTCHA'
  frame.style.cssText = 'border:0;width:100%;max-width:520px;height:140px;background:white'
  frame.referrerPolicy = 'no-referrer'
  const retry = document.createElement('button')
  retry.textContent = 'Reload CAPTCHA'
  retry.type = 'button'
  overlay.append(title, notice, frame, retry)

  let listener
  const cleanup = () => {
    window.removeEventListener('message', listener)
    overlay.remove()
  }
  const promise = new Promise(resolve => {
    listener = event => {
      if (event.origin !== 'https://www.fbsbx.com' || event.source !== frame.contentWindow) return
      const data = event.data
      if (!data || typeof data !== 'object') return
      if (data.type === 'GET_ORIGIN') {
        event.source.postMessage({}, event.origin)
      } else if (data.type === 'RESIZE_IFRAME' && Number.isFinite(data.size?.height)) {
        frame.style.height = Math.min(900, Math.max(100, data.size.height)) + 'px'
      } else if (data.type === 'CAPTCHA_EXPIRED') {
        notice.textContent = 'The CAPTCHA expired. Reload it to try again.'
      } else if (data.type === 'CAPTCHA_SOLVED' && typeof data.token === 'string' &&
        data.token.trim() && data.token.length <= 16384) {
        window.removeEventListener('message', listener)
        notice.textContent = 'CAPTCHA completed. Returning to your login…'
        retry.disabled = true
        resolve({ captcha_token: data.token })
      }
    }
    window.addEventListener('message', listener)
    retry.onclick = () => {
      notice.textContent = 'Solve the CAPTCHA below to continue your existing login.'
      frame.src = config.iframeURL
    }
    frame.src = config.iframeURL
  })
  window.__mautrixInstagramCaptcha = { instance: config.instance, promise, cleanup }
  document.body.append(overlay)
  return promise
}

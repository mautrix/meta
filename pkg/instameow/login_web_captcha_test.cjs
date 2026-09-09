// Offline webview contract test; no provider requests or CAPTCHA solving.
const assert = require('node:assert/strict')
const fs = require('node:fs')
const path = require('node:path')
const vm = require('node:vm')

async function main() {
  const listeners = new Set()
  const elements = []
  const document = {
    createElement(tag) {
      const element = { tag, style: {}, children: [], append(...nodes) { this.children.push(...nodes) }, remove() { this.removed = true } }
      if (tag === 'iframe') element.contentWindow = { postMessage(data, origin) { element.reply = { data, origin } } }
      elements.push(element)
      return element
    },
  }
  document.body = document.createElement('body')
  const window = {
    addEventListener(type, listener) { assert.equal(type, 'message'); listeners.add(listener) },
    removeEventListener(type, listener) { assert.equal(type, 'message'); listeners.delete(listener) },
  }
  const location = { origin: 'https://www.instagram.com' }
  const run = vm.runInNewContext('(' + fs.readFileSync(path.join(__dirname, 'login_web_captcha.js'), 'utf8') + ')', { document, window, location })
  const config = { instance: 'fixture-1', iframeURL: 'https://www.fbsbx.com/captcha/recaptcha/iframe/?locale=en_US' }
  const promise = run(config)
  assert.equal(run(config), promise, 'navigation reinjection must reuse pending challenge')
  let settled = false
  promise.then(() => { settled = true })
  const frame = elements.find(element => element.tag === 'iframe')
  const send = (data, origin = 'https://www.fbsbx.com', source = frame.contentWindow) => {
    for (const listener of listeners) listener({ data, origin, source })
  }
  send({ type: 'CAPTCHA_SOLVED', token: 'forged' }, 'https://www.fbsbx.com.attacker.invalid')
  send({ type: 'CAPTCHA_SOLVED', token: 'wrong-frame' }, undefined, {})
  send({ type: 'CAPTCHA_SOLVED', token: '' })
  await Promise.resolve()
  assert.equal(settled, false)
  send({ type: 'GET_ORIGIN' })
  assert.equal(frame.reply.origin, 'https://www.fbsbx.com')
  send({ type: 'RESIZE_IFRAME', size: { height: 10000 } })
  assert.equal(frame.style.height, '900px')
  send({ type: 'CAPTCHA_EXPIRED' })
  elements.find(element => element.tag === 'button').onclick()
  assert.equal(frame.src, config.iframeURL)
  send({ type: 'CAPTCHA_SOLVED', token: 'human-solution' })
  assert.equal(JSON.stringify(await promise), '{"captcha_token":"human-solution"}')
  assert.equal(listeners.size, 0)
  assert.equal(elements.find(element => element.tag === 'button').disabled, true)
  const oldOverlay = document.body.children[0]
  run({ ...config, instance: 'fixture-2' })
  assert.equal(oldOverlay.removed, true)
  assert.equal(listeners.size, 1)
  location.origin = 'https://attacker.invalid'
  await assert.rejects(run(config), /Instagram origin/)
  console.log('PASS: visible iframe, reinjection, origin and frame binding, expiry, resize, one token and fresh challenge')
}
main().catch(error => { console.error(error); process.exitCode = 1 })

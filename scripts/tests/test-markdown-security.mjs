import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import vm from 'node:vm';

// Exercise the shipped browser bundle, not just node_modules: updating the
// package without replacing web/vendor must not leave the vulnerable parser in
// the desktop executable. A VM timeout bounds pathological input regressions.
const context = vm.createContext({});
vm.runInContext(readFileSync(new URL('../../web/vendor/markdown-it.js', import.meta.url), 'utf8'), context);
vm.runInContext('parser = markdownit(); htmlParser = markdownit({ html: true });', context);
const render = (text, html = false) => {
  context.input = text;
  return vm.runInContext(`${html ? 'htmlParser' : 'parser'}.render(input)`, context, { timeout: 2000 });
};
assert.match(render('**安全预览** [链接](https://example.com)'), /<strong>安全预览<\/strong>/);
assert.doesNotMatch(render('<img src=x onerror=alert(1)> [x](javascript:alert(1))'), /<img|href="javascript:/i);
// GHSA-gfr2-rv44-r23c: unterminated reference label/title across many lines.
for (const text of ['[' + 'x\n'.repeat(100000), '[x]: /url "' + 'x\n'.repeat(100000)]) {
  assert.equal(typeof render(text), 'string');
}
// GHSA-vfgh-jcmw-g37q: repeated unterminated inline HTML constructs.
for (const opener of ['<!--', '<?', '<!A', '<![CDATA[']) {
  assert.equal(typeof render('x ' + opener.repeat(50000), true), 'string');
}
console.log('PASS: 随包 Markdown 解析器正常渲染、拒绝脚本链接，恶意引用与 HTML 输入均在限时内完成。');

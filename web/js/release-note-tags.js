'use strict';
// Apply the same category emphasis to plain-text update offers and rendered logs.
window.DengReleaseNoteTags = (() => {
  const kinds = { 新增: 'added', 修复: 'fixed', 调整: 'changed' };
  function highlight(container) {
    const walker = document.createTreeWalker(container, NodeFilter.SHOW_TEXT), texts = [];
    while (walker.nextNode()) {
      if (!walker.currentNode.parentElement.closest('pre,code,.release-note-tag')) texts.push(walker.currentNode);
    }
    for (const text of texts) {
      const matches = [...text.data.matchAll(/(^|[\r\n])([ \t]*(?:(?:[-*+]|\d+[.)])[ \t]+)?)(新增|修复|调整)([：:])/g)];
      if (!matches.length) continue;
      const fragment = document.createDocumentFragment();
      let offset = 0;
      for (const match of matches) {
        const start = match.index + match[1].length + match[2].length;
        fragment.append(document.createTextNode(text.data.slice(offset, start)));
        const tag = document.createElement('span');
        tag.className = 'release-note-tag'; tag.dataset.kind = kinds[match[3]];
        tag.textContent = match[3] + match[4]; fragment.append(tag);
        offset = match.index + match[0].length;
      }
      fragment.append(document.createTextNode(text.data.slice(offset)));
      text.replaceWith(fragment);
    }
  }
  return { highlight };
})();

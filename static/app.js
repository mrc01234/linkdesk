document.querySelectorAll('[data-copy]').forEach(button => {
  button.addEventListener('click', async () => {
    const text = button.dataset.copy;
    try {
      if (navigator.clipboard && window.isSecureContext) {
        await navigator.clipboard.writeText(text);
      } else {
        const input = document.createElement('textarea');
        input.value = text; input.style.position = 'fixed'; input.style.opacity = '0';
        document.body.appendChild(input); input.select();
        const copied = document.execCommand('copy'); input.remove();
        if (!copied) throw new Error('clipboard unavailable');
      }
      const label = button.textContent;
      button.textContent = '已复制';
      setTimeout(() => { button.textContent = label; }, 1600);
    } catch (_) { window.prompt('请复制下面的链接：', text); }
  });
});
document.querySelectorAll('form[data-confirm]').forEach(form => {
  form.addEventListener('submit', event => {
    if (!window.confirm(form.dataset.confirm)) event.preventDefault();
  });
});

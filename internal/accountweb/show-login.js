// Runs only in the account window's main frame, in WebKit's isolated client world.
(() => {
  if (location.origin !== 'https://hongguoduanju.com' || location.pathname !== '/') return;

  function showLogin() {
    if (document.querySelector('dialog[open], [role="dialog"]')) return true;
    const buttons = document.querySelectorAll('header button');
    const login = Array.from(buttons).find(button =>
      button.textContent.trim() === '登录' && !button.disabled && button.getClientRects().length
    );
    if (!login) return false;
    login.click();
    return true;
  }

  if (showLogin()) return;
  const observer = new MutationObserver(() => {
    if (showLogin()) observer.disconnect();
  });
  observer.observe(document.documentElement, { childList: true, subtree: true, characterData: true });
  setTimeout(() => observer.disconnect(), 12000);
})();

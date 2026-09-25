(() => {
  const backdrop = document.querySelector('.tenant-finder-backdrop');
  const drawer = backdrop?.querySelector('.tenant-finder-drawer');
  if (!drawer) return;
  const title = drawer.querySelector('#tenant-finder-title');
  const close = drawer.querySelector('.drawer-close');
  title?.focus({preventScroll: true});
  backdrop.addEventListener('click', event => {
    if (event.target === backdrop) close?.click();
  });
  document.addEventListener('keydown', event => {
    if (event.key === 'Escape') {
      close?.click();
      return;
    }
    if (event.key !== 'Tab') return;
    const controls = [...drawer.querySelectorAll('a[href], button:not([disabled]), input:not([type=hidden]):not([disabled])')]
      .filter(element => element.getClientRects().length);
    if (!controls.length) return;
    const first = controls[0], last = controls[controls.length - 1];
    if (event.shiftKey && (document.activeElement === first || document.activeElement === title)) {
      event.preventDefault();
      last.focus();
    } else if (!event.shiftKey && document.activeElement === last) {
      event.preventDefault();
      first.focus();
    }
  });
})();

(() => {
  const dialog = document.getElementById("danger-confirm-dialog");
  if (!dialog) return;

  const title = dialog.querySelector("#danger-confirm-title");
  const message = dialog.querySelector("#danger-confirm-message");
  const accept = dialog.querySelector("[data-confirm-accept]");
  const cancel = dialog.querySelector("[data-confirm-cancel]");
  const bypass = new WeakMap();
  let pending = null;

  const close = (accepted) => {
    if (!pending) return;
    pending.accepted = accepted;
    dialog.close();
  };

  dialog.addEventListener("close", () => {
    if (!pending) return;
    const { resolve, trigger, accepted } = pending;
    pending = null;
    if (trigger?.isConnected) trigger.focus();
    resolve(accepted);
  });
  cancel.addEventListener("click", () => close(false));
  accept.addEventListener("click", () => close(true));
  dialog.addEventListener("cancel", () => { if (pending) pending.accepted = false; });

  // Page scripts can use this for actions that do not submit a form.
  window.RentOpsConfirm = {
    open({ title: heading = "确认操作", message: detail = "请确认是否继续此操作。", confirmLabel = "确认操作", trigger = document.activeElement } = {}) {
      if (pending || dialog.open) return Promise.resolve(false);
      title.textContent = heading;
      message.textContent = detail;
      accept.textContent = confirmLabel;
      return new Promise((resolve) => {
        pending = { resolve, trigger, accepted: false };
        dialog.showModal();
        cancel.focus();
      });
    },
  };

  // A submit event means browser constraint validation has already passed.
  // Re-submit with the original button to preserve formaction and name/value.
  document.addEventListener("submit", (event) => {
    const form = event.target;
    const submitter = event.submitter;
    if (!(form instanceof HTMLFormElement)) return;
    if (bypass.get(form) === submitter && bypass.has(form)) {
      bypass.delete(form);
      return;
    }
    if (!submitter || submitter.dataset.confirm !== "true") return;
    event.preventDefault();
    if (pending) return;
    window.RentOpsConfirm.open({
      title: submitter.dataset.confirmTitle || "确认操作",
      message: submitter.dataset.confirmMessage || "此操作会修改工作区数据，请确认是否继续。",
      confirmLabel: submitter.dataset.confirmLabel || submitter.textContent.trim(),
      trigger: submitter,
    }).then((accepted) => {
      if (!accepted || !form.isConnected || !submitter.isConnected) return;
      bypass.set(form, submitter);
      try { form.requestSubmit(submitter); } finally { bypass.delete(form); }
    });
  });
})();

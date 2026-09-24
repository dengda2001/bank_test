(() => {
  const refresh = (root) => {
    if (!root) return;
    const month = root.querySelector(root.dataset.tenantMonthSelector || 'input[type="month"]')?.value;
    const targetRoom = Number(root.dataset.tenantTargetRoom || 0);
    const conflicts = new Map();
    for (const select of root.querySelectorAll('select[data-tenant-availability-select], select[name="tenant_id"]')) {
      for (const option of select.options) {
        if (!option.value) continue;
        let intervals = [];
        try { intervals = JSON.parse(option.dataset.occupancies || "[]"); } catch (_) { /* The server remains authoritative. */ }
        const conflict = intervals.find((item) => Number(item.room_id) !== targetRoom && (!item.to || item.to >= month));
        option.disabled = Boolean(conflict);
        option.dataset.tenantConflict = conflict ? "true" : "false";
        option.textContent = option.dataset.tenantName + (conflict ? "（已入住 " + conflict.room_label + "，请先解绑）" : "");
        if (conflict) conflicts.set(option.value, {name: option.dataset.tenantName, room: conflict});
      }
      const selected = select.selectedOptions[0];
      select.setCustomValidity(selected?.disabled ? "该租客已在其他房间入住，请先去原房间解绑。" : "");
    }
    const notice = root.querySelector("[data-tenant-conflicts]");
    if (!notice) return;
    notice.replaceChildren();
    notice.hidden = conflicts.size === 0;
    if (!conflicts.size) return;
    const heading = document.createElement("strong");
    heading.textContent = "以下租客已入住其他房间，请先去原房间解绑：";
    notice.append(heading);
    for (const {name, room} of conflicts.values()) {
      const row = document.createElement("p");
      row.append(document.createTextNode(name + " · " + room.room_label + "　"));
      const link = document.createElement("a");
      link.href = "/rooms/" + encodeURIComponent(room.room_id) + "?period=" + encodeURIComponent(month) + "&rent=1";
      link.textContent = "去解绑";
      row.append(link);
      notice.append(row);
    }
  };
  const start = () => {
    for (const root of document.querySelectorAll("[data-tenant-availability]")) {
      const month = root.querySelector(root.dataset.tenantMonthSelector || 'input[type="month"]');
      month?.addEventListener("change", () => refresh(root));
      month?.addEventListener("input", () => refresh(root));
	  const rows = root.querySelector("[data-plan-member-rows]");
	  if (rows) new MutationObserver(() => refresh(root)).observe(rows, {childList: true});
      refresh(root);
    }
  };
  window.refreshRoomTenantAvailability = refresh;
  if (document.readyState === "loading") document.addEventListener("DOMContentLoaded", start, {once: true});
  else start();
})();

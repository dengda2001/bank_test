(function () {
  if (window.__rentOpsWorkspaceControlsReady) return;
  window.__rentOpsWorkspaceControlsReady = true;

  let activeSelect = null;
  let selectSequence = 0;
  let typeaheadBuffer = "";
  let typeaheadTimer = 0;

  const selectSelector = "select:not([multiple]):not([data-workspace-select-ready])";
  const searchSelector = 'input[type="search"]:not([data-workspace-search-ready])';
  const tenantPeriodMatchSelector = 'form[data-tenant-period-match]:not([data-tenant-period-match-ready])';
  const textNode = (value) => document.createTextNode(value || "");

  const getLabelText = (select) => {
    const explicit = select.getAttribute("aria-label");
    if (explicit) return explicit.trim();
    const labels = Array.from(select.labels || []);
    for (const label of labels) {
      const copy = label.cloneNode(true);
      copy.querySelectorAll("select, input, button, textarea").forEach((node) => node.remove());
      const text = copy.textContent.replace(/\s+/g, " ").trim();
      if (text) return text;
    }
    return "选择选项";
  };

  const copySelectLayout = (select, wrapper, trigger) => {
    const style = window.getComputedStyle(select);
    ["width", "minWidth", "maxWidth", "flex", "alignSelf", "gridColumn", "boxSizing"].forEach((property) => {
      const value = style[property];
      if (value && value !== "auto") wrapper.style[property] = value;
    });
    ["minHeight", "height", "border", "borderRadius", "color", "backgroundColor", "fontFamily", "fontSize", "fontWeight", "lineHeight", "boxSizing", "paddingTop", "paddingBottom", "paddingLeft", "textAlign"].forEach((property) => {
      const value = style[property];
      if (value && value !== "auto") trigger.style[property] = value;
    });
    const rightPadding = Number.parseFloat(style.paddingRight) || 0;
    trigger.style.paddingRight = Math.max(rightPadding + 25, 36) + "px";
    trigger.style.width = "100%";
    trigger.style.minWidth = "0";
    trigger.style.flex = "1 1 auto";
    wrapper.style.marginTop = style.marginTop;
    wrapper.style.marginRight = style.marginRight;
    wrapper.style.marginBottom = style.marginBottom;
    wrapper.style.marginLeft = style.marginLeft;
  };

  const optionIsDisabled = (option) => option.disabled || option.hidden || Boolean(option.parentElement && option.parentElement.disabled);

  const initSelect = (select) => {
    if (select.dataset.workspaceSelectReady || select.multiple) return;
    const label = getLabelText(select);
    const labels = Array.from(select.labels || []);
    const parent = select.parentNode;
    if (!parent) return;

    select.dataset.workspaceSelectReady = "true";
    const wrapper = document.createElement("span");
    wrapper.className = "workspace-select";
    const trigger = document.createElement("button");
    trigger.type = "button";
    trigger.className = "workspace-select-trigger";
    trigger.setAttribute("role", "combobox");
    trigger.setAttribute("aria-haspopup", "listbox");
    trigger.setAttribute("aria-expanded", "false");
    trigger.setAttribute("aria-label", label);
    trigger.setAttribute("aria-required", String(select.required));
    if (select.getAttribute("aria-describedby")) trigger.setAttribute("aria-describedby", select.getAttribute("aria-describedby"));
    if (select.getAttribute("aria-labelledby")) trigger.setAttribute("aria-labelledby", select.getAttribute("aria-labelledby"));
    const value = document.createElement("span");
    value.className = "workspace-select-value";
    value.setAttribute("aria-hidden", "true");
    trigger.appendChild(value);

    const popup = document.createElement("div");
    popup.className = "workspace-select-popup";
    popup.hidden = true;
    const searchable = select.hasAttribute("data-searchable");
    const listbox = document.createElement("div");
    listbox.className = "workspace-select-options";
    listbox.id = select.id ? select.id + "-options" : "workspace-select-options-" + (++selectSequence);
    listbox.setAttribute("role", "listbox");
    listbox.setAttribute("aria-label", label);
    trigger.setAttribute("aria-controls", listbox.id);

    let searchInput = null;
    let noResults = null;
    if (searchable) {
      searchInput = document.createElement("input");
      searchInput.type = "search";
      searchInput.className = "workspace-select-search";
      searchInput.setAttribute("role", "searchbox");
      searchInput.setAttribute("aria-label", "搜索" + label.replace(/^(选择|修改)/, ""));
      searchInput.placeholder = "输入关键词搜索";
      searchInput.autocomplete = "off";
      // The shared search-clear enhancement is for page filters, not this popup.
      searchInput.dataset.workspaceSearchReady = "true";
      popup.appendChild(searchInput);
      noResults = document.createElement("div");
      noResults.className = "workspace-select-empty";
      noResults.setAttribute("role", "status");
      noResults.textContent = "没有匹配的租客";
    }
    popup.appendChild(listbox);
    if (noResults) popup.appendChild(noResults);

    let clearButton = null;
    if (select.hasAttribute("data-clearable")) {
      wrapper.classList.add("is-clearable");
      clearButton = document.createElement("button");
      clearButton.type = "button";
      clearButton.className = "workspace-select-clear";
      clearButton.setAttribute("aria-label", "清除选择");
      clearButton.title = "清除选择";
      clearButton.textContent = "×";
    }

    copySelectLayout(select, wrapper, trigger);
    parent.insertBefore(wrapper, select);
    wrapper.appendChild(select);
    wrapper.appendChild(trigger);
    if (clearButton) wrapper.appendChild(clearButton);
    document.body.appendChild(popup);
    select.classList.add("workspace-select-native");
    select.tabIndex = -1;
    select.setAttribute("aria-hidden", "true");
    trigger.disabled = select.disabled;

    let optionNodes = [];
    const selectedIndex = () => Math.max(0, Array.from(select.options).findIndex((option) => option.selected));
    const updateSelectedValue = () => {
      const option = select.selectedOptions && select.selectedOptions[0];
      value.textContent = option ? option.textContent.trim() : "";
      trigger.disabled = select.disabled;
      if (select.disabled) trigger.setAttribute("aria-disabled", "true");
      else trigger.removeAttribute("aria-disabled");
      if (select.getAttribute("aria-invalid") === "true") trigger.setAttribute("aria-invalid", "true");
      else trigger.removeAttribute("aria-invalid");
      if (clearButton) {
        clearButton.hidden = !select.value;
        clearButton.disabled = select.disabled;
      }
    };

    const applySearchFilter = () => {
      const query = searchInput ? searchInput.value.trim().toLocaleLowerCase() : "";
      let visibleCount = 0;
      optionNodes.forEach((item, index) => {
        const option = select.options[index];
        const labelText = option ? option.textContent.trim() : "";
        const matches = Boolean(option && !option.hidden && (!query || (option.value !== "" && labelText.toLocaleLowerCase().includes(query))));
        item.hidden = !matches;
        if (matches) visibleCount += 1;
      });
      if (noResults) noResults.hidden = visibleCount > 0;
      listbox.hidden = visibleCount === 0;
      if (!popup.hidden && activeSelect && activeSelect.select === select) placePopup();
    };

    const renderOptions = () => {
      listbox.replaceChildren();
      optionNodes = Array.from(select.options).map((option, index) => {
        const item = document.createElement("div");
        item.className = "workspace-select-option";
        item.setAttribute("role", "option");
        item.setAttribute("tabindex", "-1");
        item.setAttribute("data-option-index", String(index));
        item.setAttribute("aria-selected", String(option.selected));
        item.setAttribute("aria-disabled", String(optionIsDisabled(option)));
        item.appendChild(textNode(option.textContent.trim()));
        listbox.appendChild(item);
        return item;
      });
      updateSelectedValue();
      applySearchFilter();
    };

    const placePopup = () => {
      if (popup.hidden) return;
      const rect = trigger.getBoundingClientRect();
      const viewportPadding = 8;
      const availableHeight = Math.max(96, window.innerHeight - viewportPadding * 2);
      const desiredHeight = Math.min(popup.scrollHeight, 320, availableHeight);
      const below = window.innerHeight - rect.bottom - viewportPadding;
      const above = rect.top - viewportPadding;
      const openBelow = below >= desiredHeight || below >= above;
      const maxHeight = Math.max(96, Math.min(320, openBelow ? below : above));
      const actualHeight = Math.min(desiredHeight, maxHeight);
      const top = openBelow
        ? rect.bottom + 4
        : Math.max(viewportPadding, rect.top - actualHeight - 4);
      const width = Math.min(Math.max(rect.width, 120), window.innerWidth - viewportPadding * 2);
      const left = Math.min(Math.max(rect.left, viewportPadding), window.innerWidth - width - viewportPadding);
      popup.style.top = Math.round(top) + "px";
      popup.style.left = Math.round(left) + "px";
      popup.style.width = Math.round(width) + "px";
      popup.style.maxHeight = Math.round(maxHeight) + "px";
    };

    const close = (restoreFocus) => {
      popup.hidden = true;
      trigger.setAttribute("aria-expanded", "false");
      wrapper.classList.remove("is-open");
      if (activeSelect && activeSelect.select === select) activeSelect = null;
      if (restoreFocus) trigger.focus();
    };

    const focusOption = (index, direction) => {
      if (!optionNodes.length) return;
      let next = Math.min(Math.max(index, 0), optionNodes.length - 1);
      const step = direction || (index >= selectedIndex() ? 1 : -1);
      while (next >= 0 && next < optionNodes.length && (optionIsDisabled(select.options[next]) || optionNodes[next].hidden)) next += step;
      if (next < 0 || next >= optionNodes.length) {
        next = optionNodes.findIndex((node, optionIndex) => !optionIsDisabled(select.options[optionIndex]) && !node.hidden);
      }
      if (next < 0) return;
      optionNodes.forEach((node) => node.classList.remove("is-active"));
      optionNodes[next].classList.add("is-active");
      optionNodes[next].focus({ preventScroll: true });
      optionNodes[next].scrollIntoView({ block: "nearest" });
    };

    const open = (focusCurrent) => {
      if (select.disabled) return;
      if (activeSelect && activeSelect.select !== select) activeSelect.close(false);
      renderOptions();
      popup.hidden = false;
      wrapper.classList.add("is-open");
      trigger.setAttribute("aria-expanded", "true");
      activeSelect = { select, wrapper, popup, close: (restoreFocus) => close(restoreFocus), placePopup };
      placePopup();
      if (searchInput) {
        searchInput.value = "";
        applySearchFilter();
        requestAnimationFrame(() => searchInput.focus());
      } else if (focusCurrent) {
        requestAnimationFrame(() => focusOption(selectedIndex()));
      }
    };

    const choose = (index) => {
      const option = select.options[index];
      if (!option || optionIsDisabled(option)) return;
      select.selectedIndex = index;
      select.removeAttribute("aria-invalid");
      updateSelectedValue();
      optionNodes.forEach((node, optionIndex) => {
        node.setAttribute("aria-selected", String(optionIndex === index));
        node.classList.remove("is-active");
      });
      close(false);
      select.dispatchEvent(new Event("input", { bubbles: true }));
      select.dispatchEvent(new Event("change", { bubbles: true }));
      trigger.focus();
    };

    const chooseTypeahead = (character) => {
      window.clearTimeout(typeaheadTimer);
      typeaheadBuffer += character.toLocaleLowerCase();
      typeaheadTimer = window.setTimeout(() => { typeaheadBuffer = ""; }, 650);
      const options = Array.from(select.options);
      const start = Math.max(0, selectedIndex() + 1);
      for (let offset = 0; offset < options.length; offset += 1) {
        const index = (start + offset) % options.length;
        if (!optionIsDisabled(options[index]) && options[index].textContent.trim().toLocaleLowerCase().startsWith(typeaheadBuffer)) {
          choose(index);
          return;
        }
      }
    };

    trigger.addEventListener("click", () => {
      if (popup.hidden) open(true);
      else close(false);
    });
    clearButton?.addEventListener("click", () => {
      const blankIndex = Array.from(select.options).findIndex((option) => option.value === "" && !option.disabled);
      if (blankIndex < 0 || select.disabled) return;
      select.selectedIndex = blankIndex;
      select.removeAttribute("aria-invalid");
      updateSelectedValue();
      select.dispatchEvent(new Event("input", { bubbles: true }));
      select.dispatchEvent(new Event("change", { bubbles: true }));
      trigger.focus();
    });
    trigger.addEventListener("keydown", (event) => {
      if (event.key === "ArrowDown" || event.key === "ArrowUp") {
        event.preventDefault();
        if (popup.hidden) open(true);
        else focusOption(selectedIndex() + (event.key === "ArrowDown" ? 1 : -1), event.key === "ArrowDown" ? 1 : -1);
      } else if (event.key === "Enter" || event.key === " ") {
        event.preventDefault();
        if (popup.hidden) open(true);
        else close(false);
      } else if (event.key === "Escape" && !popup.hidden) {
        event.preventDefault();
        close(true);
      } else if (event.key.length === 1 && !event.ctrlKey && !event.metaKey && !event.altKey) {
        chooseTypeahead(event.key);
      }
    });
    popup.addEventListener("click", (event) => {
      const item = event.target.closest('[role="option"]');
      if (item && popup.contains(item)) choose(Number(item.dataset.optionIndex));
    });
    popup.addEventListener("pointermove", (event) => {
      const item = event.target.closest('[role="option"]');
      if (!item || !popup.contains(item) || item.getAttribute("aria-disabled") === "true") return;
      optionNodes.forEach((node) => node.classList.toggle("is-active", node === item));
    });
    searchInput?.addEventListener("input", applySearchFilter);
    searchInput?.addEventListener("keydown", (event) => {
      if (event.key === "Escape") {
        event.preventDefault();
        close(true);
      } else if (event.key === "ArrowDown" || event.key === "ArrowUp") {
        const available = optionNodes.map((node, index) => ({ node, index })).filter(({ node, index }) => !node.hidden && !optionIsDisabled(select.options[index]));
        if (!available.length) return;
        event.preventDefault();
        const target = event.key === "ArrowDown" ? available[0].index : available[available.length - 1].index;
        focusOption(target, event.key === "ArrowDown" ? 1 : -1);
      } else if (event.key === "Enter") {
        event.preventDefault();
        const activeIndex = optionNodes.findIndex((node, index) => node.classList.contains("is-active") && !node.hidden && !optionIsDisabled(select.options[index]));
        const firstAvailable = optionNodes.findIndex((node, index) => !node.hidden && !optionIsDisabled(select.options[index]));
        choose(activeIndex >= 0 ? activeIndex : firstAvailable);
      }
    });
    listbox.addEventListener("keydown", (event) => {
      const focusedIndex = Number(event.target.dataset.optionIndex);
      if (!Number.isInteger(focusedIndex)) return;
      if (event.key === "ArrowDown" || event.key === "ArrowUp") {
        event.preventDefault();
        focusOption(focusedIndex + (event.key === "ArrowDown" ? 1 : -1), event.key === "ArrowDown" ? 1 : -1);
      } else if (event.key === "Home") {
        event.preventDefault();
        focusOption(0, 1);
      } else if (event.key === "End") {
        event.preventDefault();
        focusOption(optionNodes.length - 1, -1);
      } else if (event.key === "Enter" || event.key === " ") {
        event.preventDefault();
        choose(focusedIndex);
      } else if (event.key === "Escape") {
        event.preventDefault();
        close(true);
      } else if (event.key === "Tab") {
        close(false);
      } else if (event.key.length === 1 && !event.ctrlKey && !event.metaKey && !event.altKey) {
        chooseTypeahead(event.key);
      }
    });
    select.addEventListener("change", updateSelectedValue);
    select.addEventListener("invalid", (event) => {
      event.preventDefault();
      trigger.setAttribute("aria-invalid", "true");
      trigger.focus();
      open(true);
    });
    labels.forEach((labelNode) => {
      labelNode.addEventListener("click", (event) => {
        if (event.target.closest("button, input, select, textarea, a")) return;
        event.preventDefault();
        trigger.focus();
      });
    });
    const optionObserver = new MutationObserver(renderOptions);
    optionObserver.observe(select, { childList: true, subtree: true, attributes: true, characterData: true });
    renderOptions();
  };

  const initSearchClear = (input) => {
    if (input.dataset.workspaceSearchReady) return;
    const parent = input.parentNode;
    if (!parent) return;
    input.dataset.workspaceSearchReady = "true";
    const wrapper = document.createElement("span");
    wrapper.className = "workspace-search-control";
    const clear = document.createElement("button");
    clear.type = "button";
    clear.className = "workspace-search-clear";
    clear.setAttribute("aria-label", "清除搜索内容");
    clear.title = "清除搜索内容";
    clear.textContent = "×";
    clear.hidden = !input.value;
    parent.insertBefore(wrapper, input);
    wrapper.appendChild(input);
    wrapper.appendChild(clear);
    const update = () => { clear.hidden = !input.value; };
    clear.addEventListener("click", () => {
      input.value = "";
      update();
      input.dispatchEvent(new Event("input", { bubbles: true }));
      input.focus();
    });
    input.addEventListener("input", update);
    input.addEventListener("keydown", (event) => {
      if (event.key === "Escape" && input.value) {
        event.preventDefault();
        clear.click();
      }
    });
  };

  const initTenantPeriodMatch = (form) => {
    if (form.dataset.tenantPeriodMatchReady) return;
    const tenant = form.querySelector('[name="tenant_id"]');
    const period = form.querySelector('[data-tenant-period-input]');
    const metadata = form.querySelector('template[data-tenant-period-options]');
    const helper = form.querySelector('[data-tenant-period-rent]');
    if (!tenant || !period || !metadata || !helper) return;
    form.dataset.tenantPeriodMatchReady = 'true';
    const options = Array.from(metadata.content.querySelectorAll('option')).map((option) => ({
      tenantID: option.dataset.tenant || '',
      period: option.value,
      label: option.dataset.label || '',
      due: option.dataset.due || '',
      remaining: option.dataset.rent || '',
    }));
    const hiddenPeriod = () => period.closest('.calendar-control')?.querySelector('input[type="hidden"][name="period"]');
    const periodValue = () => hiddenPeriod()?.value || period.value || '';
    const setPeriod = (value) => {
      const hidden = hiddenPeriod();
      if (hidden) hidden.value = value;
      period.value = value ? value.replace(/^(\d{4})-(\d{2})$/, '$1年$2月') : '';
    };
    const sync = () => {
      const matching = options.filter((option) => option.tenantID === tenant.value);
      const selected = periodValue();
      period.dataset.calendarAllowedValues = matching.map((option) => option.period).join(',');
      const match = matching.find((option) => option.period === selected);
      // This line alternates between nudging the next step and reporting the
      // selected month's amounts. The two carry different weight, so a class
      // drives the colour; without it a hint and a figure look equally loud.
      helper.classList.remove('is-set');
      if (selected && !match) {
        setPeriod('');
        helper.textContent = tenant.value ? '请选择该租客可匹配的月份。' : '先选择租客，再选择租金月份。';
        return;
      }
      if (!tenant.value) {
        helper.textContent = '先选择租客，再选择租金月份。';
      } else if (!selected) {
        helper.textContent = matching.length ? '请选择可匹配的租金月份。' : '该租客没有可直接匹配的租金月份。';
      } else {
        // Expected is the month's full rent obligation, remaining is what is
        // still outstanding. The landlord eyeballs both against the transaction
        // amount to tell whether it covers the month — remaining alone makes a
        // partly-paid month look identical to an untouched one.
        const figures = [];
        if (match.due) figures.push('应交 ' + match.due);
        if (match.remaining) figures.push('未收 ' + match.remaining);
        helper.textContent = figures.length
          ? (match.label ? match.label + '：' : '') + figures.join(' · ')
          : '该月份没有待匹配的租金。';
        helper.classList.add('is-set');
      }
    };
    tenant.addEventListener('change', sync);
    period.addEventListener('change', sync);
    sync();
  };

  const enhance = (root) => {
    if (!root || root.nodeType !== Node.ELEMENT_NODE) return;
    if (root.matches(selectSelector)) initSelect(root);
    if (root.matches(searchSelector)) initSearchClear(root);
    if (root.matches(tenantPeriodMatchSelector)) initTenantPeriodMatch(root);
    root.querySelectorAll(selectSelector).forEach(initSelect);
    root.querySelectorAll(searchSelector).forEach(initSearchClear);
    root.querySelectorAll(tenantPeriodMatchSelector).forEach(initTenantPeriodMatch);
  };

  const start = () => {
    enhance(document.documentElement);
    const observer = new MutationObserver((records) => {
      records.forEach((record) => record.addedNodes.forEach((node) => enhance(node)));
    });
    observer.observe(document.documentElement, { childList: true, subtree: true });
    window.addEventListener("resize", () => activeSelect && activeSelect.placePopup());
    document.addEventListener("scroll", () => activeSelect && activeSelect.placePopup(), true);
    document.addEventListener("pointerdown", (event) => {
      if (activeSelect && !activeSelect.wrapper.contains(event.target) && !activeSelect.popup.contains(event.target)) activeSelect.close(false);
    });
  };

  if (document.readyState === "loading") document.addEventListener("DOMContentLoaded", start, { once: true });
  else start();
})();

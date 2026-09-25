(() => {
  const drawer = document.querySelector('.transaction-review-drawer');
  if (!drawer) return;

  const form = drawer.querySelector('[data-review-batch]');
  const rows = drawer.querySelector('[data-review-drafts]');
  const error = drawer.querySelector('[data-draft-error]');
  const floatingNotice = document.querySelector('[data-review-notice]');
  const floatingNoticeText = floatingNotice?.querySelector('[data-review-notice-text]');
  const detailOverlay = document.querySelector('[data-review-detail-overlay]');
  const detailContent = detailOverlay?.querySelector('[data-review-detail-content]');
  const detailTitle = detailOverlay?.querySelector('#transaction-review-detail-title');
  const total = drawer.querySelector('[data-draft-total]');
  const remainder = drawer.querySelector('[data-draft-remainder]');
  const progressTrack = drawer.querySelector('[data-progress-track]');
  const confirmedSegment = drawer.querySelector('[data-progress-confirmed]');
  const draftSegment = drawer.querySelector('[data-progress-draft]');
  const remainingSegment = drawer.querySelector('[data-progress-remaining]');
  const confirmedPercent = drawer.querySelector('[data-progress-confirmed-percent]');
  const draftPercent = drawer.querySelector('[data-progress-draft-percent]');
  const remainingPercent = drawer.querySelector('[data-progress-remaining-percent]');
  const remember = drawer.querySelector('[data-remember-tenant]');
  const prepaymentChoice = drawer.querySelector('[data-prepayment-choice]');
  const prepaymentTenant = drawer.querySelector('[data-prepayment-tenant]');
  const prepaymentAmount = drawer.querySelector('[data-prepayment-amount]');
  const prepaymentSummary = drawer.querySelector('[data-prepayment-summary]');
  const queued = drawer.querySelector('[data-review-queued]');
  const submit = drawer.querySelector('.transaction-review-actions button[form="transaction-review-confirm"]');
  const sourceAmount = Number(drawer.dataset.sourceCents);
  const sourceRemaining = Number(drawer.dataset.remainingCents);
  const sourceAllocated = Math.max(0, sourceAmount - sourceRemaining);
  const storageKey = 'transactionReviewDraft:' + drawer.dataset.sourceId + (drawer.dataset.finderContext ? ':finder:' + drawer.dataset.finderContext : '');
  const money = cents => new Intl.NumberFormat('en-IE', {style: 'currency', currency: 'EUR'}).format(cents / 100);
  const asCents = value => {
    if (!/^\d+(?:\.\d{1,2})?$/.test(value.trim())) return NaN;
    const [whole, fraction = ''] = value.trim().split('.');
    const cents = Number(whole) * 100 + Number(fraction.padEnd(2, '0'));
    return Number.isSafeInteger(cents) ? cents : NaN;
  };
  const amountText = cents => (cents / 100).toFixed(2);
  const deferErrorText = code => ({
    transaction_not_pending: '这笔流水已不在待处理状态，无法暂不处理。请刷新后查看最新状态。',
    transaction_not_found: '这笔流水已无法找到，请刷新列表。',
    invalid_transaction_action: '暂不处理没有保存：页面没有提交完整的流水信息。请关闭匹配窗口，重新打开这笔流水后再试。',
    transaction_action_failed: '暂不处理未保存。数据库操作失败，请稍后重试。'
  })[code] || '暂不处理未保存，请刷新后重试。';
  let drafts = [];
  let lookupSequence = 0;
  let rememberTouched = false;
  let detailTrigger = null;
  let detailSequence = 0;

  function showError(message) {
    error.textContent = message;
    error.hidden = true;
    if (floatingNotice && floatingNoticeText) {
      floatingNoticeText.textContent = message;
      floatingNotice.hidden = !message;
    }
    const feedback = drawer.querySelector('[data-add-feedback]');
    if (feedback) {
      feedback.textContent = '';
      feedback.hidden = true;
    }
  }

  floatingNotice?.querySelector('[data-review-notice-close]')?.addEventListener('click', () => showError(''));

  function closeDetail() {
    if (!detailOverlay || detailOverlay.hidden) return;
    detailSequence++;
    detailOverlay.hidden = true;
    drawer.inert = false;
    detailContent.replaceChildren();
    detailTrigger?.focus({preventScroll: true});
  }

  async function openDetail(link) {
    if (!detailOverlay || !detailContent) return;
    const sequence = ++detailSequence;
    detailTrigger = link;
    detailOverlay.hidden = false;
    drawer.inert = true;
    detailContent.textContent = '正在加载流水详情…';
    detailTitle.focus({preventScroll: true});
    if (!document.querySelector('link[href="/static/css/pages/transaction-detail.css"]')) {
      const stylesheet = document.createElement('link');
      stylesheet.rel = 'stylesheet';
      stylesheet.href = '/static/css/pages/transaction-detail.css';
      document.head.append(stylesheet);
    }
    try {
      const response = await fetch(link.href, {credentials: 'same-origin'});
      if (!response.ok) throw new Error('流水详情加载失败，请重试。');
      const doc = new DOMParser().parseFromString(await response.text(), 'text/html');
      if (sequence !== detailSequence) return;
      const page = doc.querySelector('main.transaction-detail-page');
      if (!page) throw new Error('流水详情加载失败，请重试。');
      const sections = ['transaction-source-title', 'transaction-allocations-title', 'transaction-expense-title', 'transaction-history-title']
        .map(id => page.querySelector('[aria-labelledby="' + id + '"]')).filter(Boolean);
      if (!sections.length) throw new Error('流水详情内容不可用，请刷新后重试。');
      const heading = page.querySelector('.transaction-detail-head h1')?.textContent?.trim();
      detailTitle.textContent = heading ? '流水详情 · ' + heading : '流水详情';
      detailContent.replaceChildren(...sections.map(section => {
        const copy = section.cloneNode(true);
        copy.querySelectorAll('form, .transaction-detail-panel-actions, button').forEach(element => element.remove());
        copy.querySelectorAll('a').forEach(anchor => anchor.replaceWith(document.createTextNode(anchor.textContent)));
        return copy;
      }));
    } catch (cause) {
      closeDetail();
      showError(cause.message);
    }
  }

  detailOverlay?.querySelector('[data-review-detail-close]')?.addEventListener('click', closeDetail);
  detailOverlay?.addEventListener('click', event => { if (event.target === detailOverlay) closeDetail(); });

  function syncMonthButtons() {
    const panel = drawer.querySelector('.transaction-review-months');
    const tenantId = Number(panel?.dataset.tenantId);
    for (const button of panel?.querySelectorAll('[data-add-match]') || []) {
      const added = drafts.some(item => item.tenantId === tenantId && item.period === button.dataset.period);
      button.disabled = added;
      button.textContent = added ? '已加入分配' : '加入分配';
    }
  }

  function filterLocationOptions() {
    const lookup = drawer.querySelector('[data-review-lookup]');
    const property = lookup?.querySelector('[data-review-property]');
    const room = lookup?.querySelector('[data-review-room]');
    const tenant = lookup?.querySelector('#transaction-review-tenant');
    const note = lookup?.querySelector('[data-review-location-note]');
    if (!property || !room || !tenant) return;
    for (const option of room.options) {
      if (!option.value) continue;
      const visible = !property.value || option.dataset.propertyId === property.value;
      option.hidden = !visible;
      option.disabled = !visible;
    }
    if (room.value && room.selectedOptions[0]?.disabled) room.value = '';
    const occupantIDs = room.value ? new Set((room.selectedOptions[0]?.dataset.occupantIds || '').split(',').filter(Boolean)) : null;
    for (const option of tenant.options) {
      if (!option.value) continue;
      const visible = !occupantIDs || occupantIDs.has(option.value);
      option.hidden = !visible;
      option.disabled = !visible;
    }
    if (tenant.value && tenant.selectedOptions[0]?.disabled) tenant.value = '';
    if (note) note.textContent = room.value
      ? (occupantIDs.size ? '请选择该房间在所选月份的入住租客。' : '该房间在所选月份没有入住租客；请换月份或核对房间计划。')
      : '可按房产和房间查找该月入住租客，也可直接选择租客。';
  }

  function syncLocationFromTenant() {
    const lookup = drawer.querySelector('[data-review-lookup]');
    const property = lookup?.querySelector('[data-review-property]');
    const room = lookup?.querySelector('[data-review-room]');
    const tenant = lookup?.querySelector('#transaction-review-tenant');
    const note = lookup?.querySelector('[data-review-location-note]');
    if (!property || !room || !tenant || !tenant.value) return;
    const matches = [...room.options].filter(option => option.value &&
      (option.dataset.occupantIds || '').split(',').includes(tenant.value));
    const selectedRoom = matches.find(option => option.value === room.value);
    const onlyRoom = selectedRoom || (matches.length === 1 ? matches[0] : null);
    const propertyIDs = new Set(matches.map(option => option.dataset.propertyId));
    property.value = onlyRoom?.dataset.propertyId || (propertyIDs.size === 1 ? [...propertyIDs][0] : '');
    room.value = onlyRoom?.value || '';
    filterLocationOptions();
    if (note && !matches.length) note.textContent = '该租客在当前查看月份没有已登记的房间，请核对入住计划。';
    else if (note && matches.length > 1 && !onlyRoom) note.textContent = '该租客在当前月份对应多个房间，请手动选择。';
  }

  function saveDrafts() {
    sessionStorage.setItem(storageKey, JSON.stringify({
      items: drafts,
      requestKey: form.elements.request_key.value,
      rememberTenantID: remember.value,
      rememberTouched,
      usePrepayment: prepaymentChoice.checked,
      prepaymentTenantID: prepaymentTenant.value
    }));
  }

  function validate(showMessage = true) {
    let sum = 0;
    let message = '';
    if (!drafts.length) message = '请先加入至少一个租金月份。';
    for (const item of drafts) {
      const cents = asCents(item.amount);
      if (!Number.isFinite(cents) || cents <= 0) {
        if (!message) message = '每项分配都需要填写大于 €0 的金额，最多两位小数。';
        continue;
      }
      if (cents > item.maxCents) {
        if (!message) message = item.tenantName + ' · ' + item.label + ' 的金额超过该月未收。';
      }
      sum += cents;
    }
    if (!message && sum > sourceRemaining) message = '本次分配合计超过流水未分配金额。';
    const excess = sourceRemaining - sum;
    const prepaymentCents = prepaymentChoice.checked && excess > 0 ? excess : 0;
    if (!message && prepaymentChoice.checked && excess <= 0) message = '当前没有可记为预收款的余额。';
    if (!message && prepaymentChoice.checked && !drafts.some(item => String(item.tenantId) === prepaymentTenant.value)) message = '请选择预收款所属的租客。';
    prepaymentAmount.value = prepaymentCents ? amountText(prepaymentCents) : '';
    prepaymentSummary.textContent = prepaymentChoice.checked && prepaymentCents > 0
      ? money(sum) + ' 计入所选月份房租，' + money(prepaymentCents) + ' 保留为租客待分配预收款；以后手动用于租金。'
      : '选择后，多出的金额不会计入本月房租，以后可在租客详情手动使用。';
    if (!message && remember.value && !drafts.some(item => String(item.tenantId) === remember.value)) {
      message = '请选择本次分配中的一位实际付款人。';
    }
    total.textContent = money(sum + prepaymentCents);
    remainder.textContent = money(sourceRemaining - sum - prepaymentCents);
    const confirmedShare = sourceAmount > 0 ? Math.min(1, sourceAllocated / sourceAmount) : 0;
    const draftShare = sourceAmount > 0 ? Math.min(1 - confirmedShare, (sum + prepaymentCents) / sourceAmount) : 0;
    const remainingShare = Math.max(0, 1 - confirmedShare - draftShare);
    confirmedSegment.style.width = (confirmedShare * 100) + '%';
    draftSegment.style.width = (draftShare * 100) + '%';
    remainingSegment.style.width = (remainingShare * 100) + '%';
    confirmedPercent.textContent = Math.round(confirmedShare * 100) + '%';
    draftPercent.textContent = Math.round(draftShare * 100) + '%';
    remainingPercent.textContent = Math.round(remainingShare * 100) + '%';
    progressTrack.classList.toggle('is-over', sum > sourceRemaining);
    progressTrack.setAttribute('aria-label', '已确认 ' + money(sourceAllocated) + '；本次待确认 ' + money(sum + prepaymentCents) + '；' + (sum > sourceRemaining ? '超出可分配金额 ' + money(sum - sourceRemaining) : '确认后未分配 ' + money(sourceRemaining - sum - prepaymentCents)));
    queued.textContent = drafts.length ? '已加入 ' + drafts.length + ' 项 · 本次 ' + money(sum + prepaymentCents) : '还未加入分配';
    remainder.classList.toggle('is-negative', sum > sourceRemaining);
    submit.disabled = Boolean(message);
    if (showMessage) showError(message);
    return !message;
  }

  function renderRows() {
    rows.replaceChildren();
    if (!drafts.length) {
      const empty = document.createElement('p');
      empty.className = 'transaction-review-empty';
      empty.textContent = '还没有待确认分配。请在上方月份卡点击「加入分配」。';
      rows.append(empty);
    }
    drafts.forEach((item, index) => {
      const row = document.createElement('div');
      row.className = 'transaction-review-draft';
      const heading = document.createElement('div');
      heading.className = 'transaction-review-draft-name';
      const name = document.createElement('strong');
      name.textContent = item.tenantName + ' · ' + item.label;
      const limit = document.createElement('small');
      limit.textContent = '本月未收 ' + money(item.maxCents);
      heading.append(name, limit);

      const amountLabel = document.createElement('label');
      amountLabel.textContent = '分配金额';
      const amount = document.createElement('input');
      amount.type = 'number';
      amount.name = 'amount[]';
      amount.min = '0.01';
      amount.max = amountText(item.maxCents);
      amount.step = '0.01';
      amount.inputMode = 'decimal';
      amount.required = true;
      amount.value = item.amount;
      amount.setAttribute('aria-label', item.tenantName + ' ' + item.label + ' 分配金额');
      amount.dataset.draftIndex = String(index);
      amountLabel.append(amount);

      const remove = document.createElement('button');
      remove.type = 'button';
      remove.className = 'btn subtle';
      remove.textContent = '移除';
      remove.setAttribute('aria-label', '移除' + item.tenantName + item.label);
      remove.dataset.removeDraft = String(index);
      const tenant = document.createElement('input');
      tenant.type = 'hidden';
      tenant.name = 'tenant_id[]';
      tenant.value = String(item.tenantId);
      const period = document.createElement('input');
      period.type = 'hidden';
      period.name = 'period[]';
      period.value = item.period;
      row.append(heading, amountLabel, remove, tenant, period);
      rows.append(row);
    });

    const selected = remember.value;
    const selectedPrepaymentTenant = prepaymentTenant.value;
    remember.replaceChildren(new Option('不保存付款人关系', ''));
    const names = new Map(drafts.map(item => [String(item.tenantId), item.tenantName]));
    for (const [id, name] of names) remember.add(new Option(name, id));
    remember.value = names.has(selected) ? selected : (!rememberTouched && names.has(drawer.dataset.rememberCandidate) ? drawer.dataset.rememberCandidate : '');
    prepaymentTenant.replaceChildren(new Option('请选择租客', ''));
    for (const [id, name] of names) prepaymentTenant.add(new Option(name, id));
    prepaymentTenant.value = names.has(selectedPrepaymentTenant) ? selectedPrepaymentTenant : (names.size === 1 ? [...names.keys()][0] : '');
    syncMonthButtons();
    validate(false);
  }

  function addMonth(button) {
    const panel = button.closest('.transaction-review-months');
    const tenantId = Number(panel?.dataset.tenantId);
    const tenantName = panel?.dataset.tenantName || '';
    const period = button.dataset.period;
    const maxCents = Number(button.dataset.remainingCents);
    if (!tenantId || !period || !Number.isSafeInteger(maxCents) || maxCents <= 0) return;
    if (drafts.some(item => item.tenantId === tenantId && item.period === period)) {
      showError('这个租客的该月份已在待确认分配中，可直接修改下方金额。');
      return;
    }
    const used = drafts.reduce((sum, item) => sum + Math.max(0, asCents(item.amount) || 0), 0);
    const available = sourceRemaining - used;
    if (available <= 0) {
      showError('流水未分配金额已用完。请先调整或移除下方分配。');
      return;
    }
    drafts.push({
      tenantId,
      tenantName,
      period,
      label: button.dataset.label || period,
      maxCents,
      amount: amountText(Math.min(available, maxCents))
    });
    renderRows();
    showError('');
    saveDrafts();
  }

  async function replaceEvidence(url, historyOnly = false) {
    const sequence = ++lookupSequence;
    const response = await fetch(url, {credentials: 'same-origin'});
    if (!response.ok) throw new Error('核对内容加载失败，请重试。');
    const doc = new DOMParser().parseFromString(await response.text(), 'text/html');
    const nextDrawer = doc.querySelector('.transaction-review-drawer');
    if (!nextDrawer) throw new Error('核对内容加载失败，请重试。');
    if (sequence !== lookupSequence) return;
    const nextHistory = nextDrawer.querySelector('.transaction-review-history');
    drawer.querySelector('.transaction-review-history').replaceWith(nextHistory);
    if (!historyOnly) {
      const nextIdentity = nextDrawer.querySelector('.transaction-review-identity');
      drawer.querySelector('.transaction-review-identity').replaceWith(nextIdentity);
      syncLocationFromTenant();
      const nextMonths = nextDrawer.querySelector('.transaction-review-months');
      drawer.querySelector('.transaction-review-months').replaceWith(nextMonths);
      form.elements.match_tenant.value = nextMonths.dataset.tenantId || '';
      syncMonthButtons();
      nextMonths.querySelector('h3')?.focus({preventScroll: true});
    }
    history.replaceState(history.state, '', url);
    showError('');
  }

  drawer.addEventListener('click', event => {
    const detailLink = event.target.closest('[data-review-detail-link]');
    if (detailLink) {
      event.preventDefault();
      openDetail(detailLink);
      return;
    }
    const add = event.target.closest('[data-add-match]');
    const finderBack = event.target.closest('[data-finder-back]');
    if (finderBack) {
      event.preventDefault();
      (async () => {
        if (drafts.length) {
          const confirmed = await window.RentOpsConfirm.open({
            title: '返回流水列表',
            message: '当前待确认分配还没有入账。返回后会清除这笔流水的草稿。',
            confirmLabel: '清除草稿并返回',
            trigger: finderBack
          });
          if (!confirmed) return;
        }
        sessionStorage.removeItem(storageKey);
        location.assign(finderBack.href);
      })().catch(cause => showError(cause.message));
      return;
    }
    if (add) {
      addMonth(add);
      return;
    }
    const remove = event.target.closest('[data-remove-draft]');
    if (remove) {
      drafts.splice(Number(remove.dataset.removeDraft), 1);
      renderRows();
      showError('');
      saveDrafts();
      return;
    }
    const historyLink = event.target.closest('.transaction-review-pagination a');
    if (historyLink) {
      event.preventDefault();
      replaceEvidence(historyLink.href, true).catch(cause => showError(cause.message));
      return;
    }
    const tenantLink = event.target.closest('[data-review-tenant-link]');
    if (tenantLink) {
      event.preventDefault();
      replaceEvidence(tenantLink.href).catch(cause => showError(cause.message));
      return;
    }
    if (event.target.closest('.transaction-review-actions a, .drawer-close')) {
      sessionStorage.removeItem(storageKey);
    }
  });

  drawer.addEventListener('input', event => {
    const input = event.target.closest('[data-draft-index]');
    if (!input) return;
    const item = drafts[Number(input.dataset.draftIndex)];
    if (!item) return;
    item.amount = input.value;
    validate();
    saveDrafts();
  });
  drawer.addEventListener('change', event => {
    if (event.target.matches('[data-review-property], [data-review-room]')) {
      filterLocationOptions();
      return;
    }
    if (!event.target.matches('#transaction-review-tenant')) return;
    syncLocationFromTenant();
    if (event.target.value === drawer.querySelector('.transaction-review-months')?.dataset.tenantId) return;
    drawer.querySelector('.transaction-review-smart-hint')?.remove();
  });
  remember.addEventListener('change', () => {
    rememberTouched = true;
    validate();
    saveDrafts();
  });
  prepaymentChoice.addEventListener('change', () => { validate(); saveDrafts(); });
  prepaymentTenant.addEventListener('change', () => { validate(); saveDrafts(); });

  drawer.addEventListener('submit', event => {
    const revokeForm = event.target.closest('[data-review-revoke-share]');
    if (revokeForm) {
      event.preventDefault();
      const button = event.submitter || revokeForm.querySelector('button[type=submit]');
      (async () => {
        const confirmed = await window.RentOpsConfirm.open({
          title: '撤销这一份匹配',
          message: revokeForm.dataset.revokeMessage,
          confirmLabel: '确认撤销这一份',
          trigger: button
        });
        if (!confirmed) return;
        button.disabled = true;
        try {
          const response = await fetch(revokeForm.action, {method: 'POST', credentials: 'same-origin', body: new FormData(revokeForm)});
          if (!response.redirected) throw new Error('撤销未完成，请重试。');
          const target = new URL(response.url);
          if (target.searchParams.has('error') || target.searchParams.get('message') !== 'allocation_revoked') {
            throw new Error('撤销未完成。该分配可能已变化，请刷新后核对。');
          }
          const current = new URL(location.href);
          current.searchParams.set('message', 'allocation_revoked');
          location.assign(current.href);
        } catch (cause) {
          button.disabled = false;
          showError(cause.message);
        }
      })().catch(cause => showError(cause.message));
      return;
    }
    const deferForm = event.target.closest('[data-review-defer]');
    if (deferForm) {
      event.preventDefault();
      const button = event.submitter || deferForm.querySelector('button[type=submit]');
      (async () => {
        const confirmed = await window.RentOpsConfirm.open({
          title: '暂不处理这笔流水',
          message: '确认后，这笔流水不会再出现在首页的待人工处理列表。后续可到流水页筛选「待处理」状态，继续匹配。',
          confirmLabel: '确认暂不处理',
          trigger: button
        });
        if (!confirmed) return;
        button.disabled = true;
        try {
          const response = await fetch(deferForm.action, {method: 'POST', credentials: 'same-origin', body: new FormData(deferForm)});
          if (!response.redirected) throw new Error('暂不处理未保存，服务未返回操作结果，请重试。');
          const target = new URL(response.url);
          if (target.searchParams.has('error')) {
            throw new Error(deferErrorText(target.searchParams.get('error')));
          }
          if (target.searchParams.get('message') !== 'transaction_action_saved') {
            throw new Error('暂不处理未保存，服务返回了意外结果，请刷新后重试。');
          }
          sessionStorage.removeItem(storageKey);
          location.assign(target.href);
        } catch (cause) {
          button.disabled = false;
          showError(cause.message);
        }
      })().catch(cause => showError(cause.message));
      return;
    }
    const lookup = event.target.closest('[data-review-lookup], [data-review-month-lookup]');
    if (lookup) {
      event.preventDefault();
      const url = new URL(lookup.action, location.href);
      for (const [key, value] of new FormData(lookup)) url.searchParams.set(key, value);
      if (lookup.matches('[data-review-month-lookup]')) url.searchParams.delete('match_origin');
      replaceEvidence(url.href).catch(cause => showError(cause.message));
      return;
    }
    if (event.target !== form) return;
    event.preventDefault();
    if (!validate()) return;
    saveDrafts();
    submit.disabled = true;
    fetch(form.action, {method: 'POST', credentials: 'same-origin', body: new FormData(form)})
      .then(async response => {
        if (!response.redirected) throw new Error('分配未保存，请重试。');
        const target = new URL(response.url);
        if (target.searchParams.has('error')) {
          const doc = new DOMParser().parseFromString(await response.text(), 'text/html');
          const notice = doc.querySelector('.transaction-review-drawer .notice.error, .notice.error');
          throw new Error(notice?.textContent.trim() || '分配未保存，请核对租金和流水余额后重试。');
        }
        if (target.searchParams.get('message') === 'rent_confirmed') {
          sessionStorage.removeItem(storageKey);
        }
        location.assign(target.href);
      })
      .catch(cause => {
        submit.disabled = false;
        showError(cause.message);
      });
  });

  try {
    const saved = JSON.parse(sessionStorage.getItem(storageKey) || 'null');
    if (saved && Array.isArray(saved.items)) {
      drafts = saved.items.filter(item => Number.isInteger(item.tenantId) && /^\d{4}-\d{2}$/.test(item.period) && Number.isSafeInteger(item.maxCents) && item.maxCents > 0);
      if (saved.requestKey) form.elements.request_key.value = saved.requestKey;
      rememberTouched = Boolean(saved.rememberTouched);
      if (saved.rememberTenantID) remember.value = saved.rememberTenantID;
      renderRows();
      prepaymentChoice.checked = Boolean(saved.usePrepayment);
      prepaymentTenant.value = saved.prepaymentTenantID || prepaymentTenant.value;
      validate(false);
    }
  } catch (_) {
    sessionStorage.removeItem(storageKey);
  }
  filterLocationOptions();
  validate(false);
})();
  if (!drafts.length && drawer.dataset.prefillPeriod) {
    const target = [...drawer.querySelectorAll('[data-add-match]')]
      .find(button => button.dataset.period === drawer.dataset.prefillPeriod);
    if (target) addMonth(target);
  }

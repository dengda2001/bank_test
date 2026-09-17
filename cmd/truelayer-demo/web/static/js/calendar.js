
(function () {
  const monthLabels = ['1月', '2月', '3月', '4月', '5月', '6月', '7月', '8月', '9月', '10月', '11月', '12月'];
  const weekdayLabels = ['日', '一', '二', '三', '四', '五', '六'];

  const pad = (value) => String(value).padStart(2, '0');
  const today = () => {
    const date = new Date();
    return { year: date.getFullYear(), month: date.getMonth() + 1, day: date.getDate() };
  };
  const parseValue = (kind, value) => {
    const parts = String(value || '').split('-').map(Number);
    if (parts.some((part) => !Number.isFinite(part))) return null;
    if (kind === 'month' && parts.length === 2 && parts[0] > 0 && parts[1] >= 1 && parts[1] <= 12) {
      return { year: parts[0], month: parts[1], day: 1 };
    }
    if (kind === 'date' && parts.length === 3 && parts[0] > 0 && parts[1] >= 1 && parts[1] <= 12 && parts[2] >= 1 && parts[2] <= 31) {
      return { year: parts[0], month: parts[1], day: parts[2] };
    }
    return null;
  };
  const formatValue = (kind, value) => {
    if (!value) return '';
    const month = pad(value.month);
    return kind === 'month' ? value.year + '-' + month : value.year + '-' + month + '-' + pad(value.day);
  };
  const formatDisplay = (kind, value) => {
    if (!value) return '';
    const month = pad(value.month);
    return kind === 'month'
      ? value.year + '年' + month + '月'
      : value.year + '年' + month + '月' + pad(value.day) + '日';
  };
  const sameValue = (left, right, kind) => left && right && left.year === right.year && left.month === right.month && (kind === 'month' || left.day === right.day);
  const sameMonth = (left, right) => left && right && left.year === right.year && left.month === right.month;
  const sameYear = (left, right) => left && right && left.year === right.year;
  const decadeStart = (year) => Math.floor(year / 10) * 10;
  const daysInMonth = (year, month) => new Date(year, month, 0).getDate();
  const moveMonth = (value, offset) => {
    const date = new Date(value.year, value.month - 1 + offset, 1);
    return { year: date.getFullYear(), month: date.getMonth() + 1, day: 1 };
  };
  const button = (className, text, label) => {
    const element = document.createElement('button');
    element.type = 'button';
    element.className = className;
    element.textContent = text;
    if (label) element.setAttribute('aria-label', label);
    return element;
  };

  function initCalendar(input) {
    if (input.dataset.calendarReady) return;
    const kind = input.type === 'month' ? 'month' : 'date';
    const originalValue = input.value;
    const originalName = input.name;
    const wrapper = document.createElement('div');
    wrapper.className = 'calendar-control';
    const hidden = document.createElement('input');
    hidden.type = 'hidden';
    hidden.name = originalName;
    hidden.value = originalValue;
    const trigger = button('calendar-trigger', '', '打开日期选择器');
    const popover = document.createElement('div');
    popover.className = 'calendar-popover';
    popover.dataset.calendarKind = kind;
    popover.setAttribute('role', 'dialog');
    popover.setAttribute('aria-label', kind === 'month' ? '选择月份' : '选择日期');

    input.dataset.calendarReady = 'true';
    input.dataset.calendarKind = kind;
    input.classList.add('calendar-input');
    input.type = 'text';
    input.name = '';
    input.readOnly = true;
    input.inputMode = 'none';
    input.placeholder = kind === 'month' ? '----年--月' : '----年--月--日';
    input.value = formatDisplay(kind, parseValue(kind, originalValue));
    input.setAttribute('aria-haspopup', 'dialog');
    input.setAttribute('aria-expanded', 'false');
    input.parentNode.insertBefore(wrapper, input);
    wrapper.appendChild(input);
    wrapper.appendChild(hidden);
    wrapper.appendChild(trigger);
    wrapper.appendChild(popover);

    // baseLevel is the innermost view for this input: the day grid for a date
    // picker, the month grid for a month picker. Every open starts there.
    const baseLevel = kind === 'month' ? 'month' : 'day';
    const state = {
      selected: parseValue(kind, originalValue),
      view: parseValue(kind, originalValue) || today(),
      level: baseLevel
    };

    const close = () => {
      wrapper.classList.remove('is-open');
      input.setAttribute('aria-expanded', 'false');
    };
    const setValue = (value, notify) => {
      hidden.value = formatValue(kind, value);
      input.value = formatDisplay(kind, value);
      state.selected = value;
      if (notify) {
        input.dispatchEvent(new Event('input', { bubbles: true }));
        input.dispatchEvent(new Event('change', { bubbles: true }));
      }
    };
    const isWithinRange = (value) => {
      const formatted = formatValue(kind, value);
      const min = input.getAttribute('min');
      const max = input.getAttribute('max');
      return (!min || formatted >= min) && (!max || formatted <= max);
    };
    const selectValue = (value) => {
      if (!isWithinRange(value)) return;
      setValue(value, true);
      close();
    };
    const renderHeader = (title, previousLabel, nextLabel, onPrevious, onNext, onTitle) => {
      const header = document.createElement('div');
      header.className = 'calendar-header';
      const previous = button('calendar-nav', '‹', previousLabel);
      const next = button('calendar-nav', '›', nextLabel);
      previous.addEventListener('click', () => { onPrevious(); render(); });
      next.addEventListener('click', () => { onNext(); render(); });
      let heading;
      if (onTitle) {
        heading = button('calendar-title', title, onTitle.label);
        heading.addEventListener('click', () => { onTitle.action(); render(); });
      } else {
        heading = document.createElement('strong');
        heading.className = 'calendar-title is-static';
        heading.textContent = title;
      }
      header.append(previous, heading, next);
      return header;
    };
    // Zooming out is always one step at a time: 日 → 月 → 年. The year level is
    // the outermost, so renderHeader is called without onTitle there and the
    // heading comes back inert.
    const zoomTo = (level, label) => ({ label, action: () => { state.level = level; } });
    const renderFooter = () => {
      const footer = document.createElement('div');
      footer.className = 'calendar-footer';
      const clear = button('calendar-action', '清除');
      const todayButton = button('calendar-action', kind === 'month' ? '本月' : '今天');
      clear.addEventListener('click', () => { setValue(null, true); close(); });
      todayButton.addEventListener('click', () => {
        const current = today();
        selectValue(kind === 'month' ? current : current);
      });
      footer.append(clear, todayButton);
      return footer;
    };
    const renderYear = () => {
      const view = state.view;
      const start = decadeStart(view.year);
      popover.appendChild(renderHeader(
        start + ' - ' + (start + 9),
        '前十年',
        '后十年',
        () => { state.view = { year: view.year - 10, month: view.month, day: 1 }; },
        () => { state.view = { year: view.year + 10, month: view.month, day: 1 }; }
      ));
      const divider = document.createElement('div');
      divider.className = 'calendar-divider';
      popover.appendChild(divider);
      const grid = document.createElement('div');
      grid.className = 'calendar-year-grid';
      for (let offset = 0; offset < 10; offset += 1) {
        const value = { year: start + offset, month: view.month, day: 1 };
        const option = button('calendar-option', String(value.year));
        if (sameYear(state.selected, value)) option.classList.add('is-selected');
        if (sameYear(today(), value)) option.classList.add('is-today');
        option.addEventListener('click', () => {
          state.view = value;
          state.level = 'month';
          render();
        });
        grid.appendChild(option);
      }
      popover.appendChild(grid);
    };
    const renderMonth = () => {
      const view = state.view;
      popover.appendChild(renderHeader(
        view.year + '年',
        '上一年',
        '下一年',
        () => { state.view = { year: view.year - 1, month: view.month, day: 1 }; },
        () => { state.view = { year: view.year + 1, month: view.month, day: 1 }; },
        zoomTo('year', '选择年份')
      ));
      const divider = document.createElement('div');
      divider.className = 'calendar-divider';
      popover.appendChild(divider);
      const grid = document.createElement('div');
      grid.className = 'calendar-month-grid';
      for (let month = 1; month <= 12; month += 1) {
        const value = { year: view.year, month, day: 1 };
        const option = button('calendar-option', monthLabels[month - 1]);
        if (sameMonth(state.selected, value)) option.classList.add('is-selected');
        if (sameMonth(today(), value)) option.classList.add('is-today');
        if (!isWithinRange(value)) option.disabled = true;
        option.addEventListener('click', () => {
          if (kind === 'month') {
            selectValue(value);
            return;
          }
          state.view = value;
          state.level = 'day';
          render();
        });
        grid.appendChild(option);
      }
      popover.appendChild(grid);
    };
    const renderDate = () => {
      const view = state.view;
      const title = view.year + '年' + pad(view.month) + '月';
      popover.appendChild(renderHeader(
        title,
        '上个月',
        '下个月',
        () => { state.view = moveMonth(view, -1); },
        () => { state.view = moveMonth(view, 1); },
        zoomTo('month', '选择月份')
      ));
      const divider = document.createElement('div');
      divider.className = 'calendar-divider';
      popover.appendChild(divider);
      const grid = document.createElement('div');
      grid.className = 'calendar-day-grid';
      weekdayLabels.forEach((label) => {
        const weekday = document.createElement('span');
        weekday.className = 'calendar-weekday';
        weekday.textContent = label;
        grid.appendChild(weekday);
      });
      const firstDay = new Date(view.year, view.month - 1, 1).getDay();
      for (let index = 0; index < firstDay; index += 1) grid.appendChild(document.createElement('span'));
      for (let day = 1; day <= daysInMonth(view.year, view.month); day += 1) {
        const value = { year: view.year, month: view.month, day };
        const option = button('calendar-option', String(day));
        if (sameValue(state.selected, value, kind)) option.classList.add('is-selected');
        if (sameValue(today(), value, kind)) option.classList.add('is-today');
        if (!isWithinRange(value)) option.disabled = true;
        option.addEventListener('click', () => selectValue(value));
        grid.appendChild(option);
      }
      popover.appendChild(grid);
    };
    const render = () => {
      popover.replaceChildren();
      if (state.level === 'year') renderYear();
      else if (state.level === 'month') renderMonth();
      else renderDate();
      popover.appendChild(renderFooter());
    };

    const open = () => {
      document.querySelectorAll('.calendar-control.is-open').forEach((element) => {
        if (element !== wrapper) element.classList.remove('is-open');
      });
      state.selected = parseValue(kind, hidden.value);
      state.view = state.selected || today();
      state.level = baseLevel;
      render();
      wrapper.classList.add('is-open');
      input.setAttribute('aria-expanded', 'true');
    };
    input.addEventListener('click', () => wrapper.classList.contains('is-open') ? close() : open());
    input.addEventListener('keydown', (event) => {
      if (event.key === 'Enter' || event.key === ' ') { event.preventDefault(); open(); }
      if (event.key === 'Escape') close();
    });
    trigger.addEventListener('click', () => wrapper.classList.contains('is-open') ? close() : open());
    // render() rebuilds the popover on every interaction, so the element a click
    // started on is already detached by the time the event bubbles up here.
    // wrapper.contains(event.target) would then be false and the picker would treat
    // its own inner clicks as outside clicks, closing itself. The propagation path
    // is snapshotted at dispatch time, so it still names the wrapper.
    const isInside = (event) => {
      if (typeof event.composedPath === 'function') return event.composedPath().includes(wrapper);
      return wrapper.contains(event.target) || !event.target.isConnected;
    };
    document.addEventListener('click', (event) => {
      if (!isInside(event)) close();
    });
    document.addEventListener('keydown', (event) => {
      if (event.key === 'Escape') close();
    });
  }

  const initCalendars = () => document.querySelectorAll('input[type="date"], input[type="month"]').forEach(initCalendar);
  if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', initCalendars);
  else initCalendars();
})();

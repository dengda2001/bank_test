package main

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"
)

const (
	dunningActionPage    = "page"
	dunningActionConfig  = "config"
	dunningActionPreview = "preview"
	dunningActionSend    = "send"
)

type dunningDrawerData struct {
	Enabled            bool
	Open               bool
	Period             string
	SearchFilter       string
	StatusFilter       string
	SortFilter         string
	Page               int
	PageSize           int
	Candidates         []dunningCandidate
	Sender             dunningSenderConfig
	SenderConfigured   bool
	ConfigurationError string
	RequestKey         string
	PreviewRows        []dunningPreview
	Results            []dunningSendResult
	Notice             string
	Error              string
}

type dunningDashboardAction struct {
	Kind           string
	Period         time.Time
	Filters        rentDashboardFilters
	SelectedIDs    []uint64
	RequestKey     string
	ForceResend    bool
	RetryAttemptID uint64
	Sender         dunningSenderConfig
}

func (a *app) handleDunningConfig(w http.ResponseWriter, r *http.Request) {
	a.handleDunningAction(w, r, dunningActionConfig)
}

func (a *app) handleDunningPreview(w http.ResponseWriter, r *http.Request) {
	a.handleDunningAction(w, r, dunningActionPreview)
}

func (a *app) handleDunningSend(w http.ResponseWriter, r *http.Request) {
	a.handleDunningAction(w, r, dunningActionSend)
}

func (a *app) handleDunningAction(w http.ResponseWriter, r *http.Request, kind string) {
	if !a.requireAuth(w, r) {
		return
	}
	if a.db == nil {
		http.Error(w, "dunning requires database-backed user sessions", http.StatusServiceUnavailable)
		return
	}
	if _, ok := a.currentUserID(r); !ok {
		http.Error(w, "dunning requires a database-backed user session", http.StatusUnauthorized)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	action, err := dunningDashboardActionFromRequest(r, kind)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	a.renderRentDashboard(w, r, action)
}

func dunningDashboardActionFromRequest(r *http.Request, kind string) (*dunningDashboardAction, error) {
	if err := r.ParseForm(); err != nil {
		return nil, err
	}
	period, err := parsePeriodMonth(strings.TrimSpace(r.Form.Get("period")))
	if err != nil {
		return nil, errors.New("dunning period is invalid")
	}
	filters, err := rentDashboardFiltersFromQuery(r.Form)
	if err != nil {
		return nil, err
	}
	if kind != dunningActionConfig && kind != dunningActionPreview && kind != dunningActionSend {
		return nil, errors.New("dunning action is invalid")
	}
	action := &dunningDashboardAction{
		Kind:        kind,
		Period:      period,
		Filters:     filters,
		SelectedIDs: make([]uint64, 0, len(r.Form["obligation_id"])),
		RequestKey:  strings.TrimSpace(r.Form.Get("request_key")),
		ForceResend: strings.TrimSpace(r.Form.Get("confirm_resend")) == "1",
		Sender: dunningSenderConfig{
			DisplayName:  strings.TrimSpace(r.Form.Get("display_name")),
			ReplyToEmail: strings.TrimSpace(r.Form.Get("reply_to_email")),
		},
	}
	for _, rawID := range r.Form["obligation_id"] {
		id, err := parsePositiveUint(rawID)
		if err != nil {
			return nil, errors.New("dunning obligation selection is invalid")
		}
		action.SelectedIDs = append(action.SelectedIDs, id)
	}
	action.SelectedIDs = uniqueDunningIDs(action.SelectedIDs)
	if len(action.SelectedIDs) > dashboardMaxPageSize {
		return nil, errors.New("too many dunning obligations selected")
	}
	if rawRetry := strings.TrimSpace(r.Form.Get("retry_of_attempt_id")); rawRetry != "" {
		action.RetryAttemptID, err = parsePositiveUint(rawRetry)
		if err != nil {
			return nil, errors.New("dunning retry selection is invalid")
		}
		if len(action.SelectedIDs) != 1 {
			return nil, errors.New("a retry must select exactly one obligation")
		}
	}
	if kind == dunningActionSend && action.RequestKey == "" {
		return nil, errors.New("dunning request key is required")
	}
	if action.RequestKey != "" {
		if err := validateDunningRequestKey(action.RequestKey); err != nil {
			return nil, err
		}
	}
	return action, nil
}

func (a *app) dunningDelivery() dunningMailDelivery {
	if a.dunningMailer != nil {
		return a.dunningMailer
	}
	return newSMTPDunningDelivery(dunningSMTPConfigFromConfig(a.cfg))
}

func (a *app) dunningDrawerForDashboard(ctx context.Context, userID uint64, period time.Time, filters rentDashboardFilters, rows []rentDashboardRow, action *dunningDashboardAction) (dunningDrawerData, error) {
	now := time.Now().UTC()
	service := newDunningService(a.db)
	candidates, err := service.listCandidatesForRows(ctx, userID, rows, now)
	if err != nil {
		return dunningDrawerData{}, err
	}
	sender, err := service.loadSenderConfig(ctx, userID)
	if err != nil {
		return dunningDrawerData{}, err
	}
	view := dunningDrawerData{
		Enabled:          true,
		Open:             action != nil,
		Period:           period.Format("2006-01"),
		SearchFilter:     filters.Search,
		StatusFilter:     filters.Status,
		SortFilter:       filters.Sort,
		Page:             filters.Page,
		PageSize:         filters.PageSize,
		Candidates:       candidates,
		Sender:           sender,
		SenderConfigured: sender.ID != 0,
		RequestKey:       recordID("dunning-request", now),
	}
	if !view.SenderConfigured {
		view.ConfigurationError = "请先保存房东发件配置；SMTP 服务发件地址还需要通过 DUNNING_SMTP_FROM 配置。"
	}
	selected := make(map[uint64]bool)
	if action != nil && action.Kind != dunningActionPage {
		for _, id := range action.SelectedIDs {
			selected[id] = true
		}
		if action.RequestKey != "" {
			view.RequestKey = action.RequestKey
		}
	} else {
		for _, candidate := range candidates {
			selected[candidate.ObligationID] = candidate.DefaultSelected
		}
	}
	for index := range view.Candidates {
		view.Candidates[index].Selected = selected[view.Candidates[index].ObligationID]
	}
	if action == nil {
		return view, nil
	}
	if action.Kind == dunningActionPage {
		return view, nil
	}

	if action.Kind == dunningActionConfig {
		saved, saveErr := service.saveSenderConfig(ctx, userID, action.Sender)
		if saveErr != nil {
			view.Error = saveErr.Error()
			view.Sender = action.Sender
			return view, nil
		}
		view.Sender = saved
		view.SenderConfigured = true
		view.ConfigurationError = ""
		view.Notice = "发件配置已保存。"
		return view, nil
	}
	if err := validateDunningPageSelection(view.Candidates, action.SelectedIDs); err != nil {
		view.Error = err.Error()
		return view, nil
	}
	switch action.Kind {
	case dunningActionPreview:
		batch, previewErr := service.preview(ctx, userID, period, action.SelectedIDs, now)
		if previewErr != nil {
			view.Error = previewErr.Error()
			return view, nil
		}
		view.Sender = batch.Sender
		view.SenderConfigured = batch.SenderConfigured
		view.ConfigurationError = batch.ConfigurationError
		view.PreviewRows = batch.Rows
	case dunningActionSend:
		serviceFrom := dunningSMTPConfigFromConfig(a.cfg).FromEmail
		results, sendErr := service.send(ctx, userID, period, action.SelectedIDs, action.RequestKey, action.ForceResend, action.RetryAttemptID, serviceFrom, a.dunningDelivery(), now)
		if sendErr != nil {
			view.Error = sendErr.Error()
			return view, nil
		}
		view.Results = results
		for index := range view.Results {
			if view.Results[index].Attempt != nil && view.Results[index].Attempt.DeliveryStatus == dunningDeliveryFailed {
				view.Results[index].RetryRequestKey = recordID("dunning-retry", now.Add(time.Duration(index+1)*time.Nanosecond))
			}
		}
	}
	return view, nil
}

func validateDunningPageSelection(candidates []dunningCandidate, selectedIDs []uint64) error {
	ids := uniqueDunningIDs(selectedIDs)
	if len(ids) == 0 || len(ids) > dashboardMaxPageSize {
		return errors.New("请选择当前页至少一条未缴账单")
	}
	allowed := make(map[uint64]struct{}, len(candidates))
	for _, candidate := range candidates {
		allowed[candidate.ObligationID] = struct{}{}
	}
	for _, id := range ids {
		if _, ok := allowed[id]; !ok {
			return errors.New("只能选择当前页的账单")
		}
	}
	return nil
}

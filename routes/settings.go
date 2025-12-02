package routes

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/duke-git/lancet/v2/condition"
	"github.com/go-chi/chi/v5"
	"github.com/gofrs/uuid/v5"
	"github.com/muety/wakapi/helpers"

	"log/slog"

	datastructure "github.com/duke-git/lancet/v2/datastructure/set"
	"github.com/gorilla/schema"
	conf "github.com/muety/wakapi/config"
	"github.com/muety/wakapi/middlewares"
	"github.com/muety/wakapi/models"
	"github.com/muety/wakapi/models/view"
	routeutils "github.com/muety/wakapi/routes/utils"
	"github.com/muety/wakapi/services"
	"github.com/muety/wakapi/services/imports"
	"github.com/muety/wakapi/utils"
)

type SettingsHandler struct {
	config              *conf.Config
	userSrvc            services.IUserService
	summarySrvc         services.ISummaryService
	heartbeatSrvc       services.IHeartbeatService
	durationSrvc        services.IDurationService
	aliasSrvc           services.IAliasService
	aggregationSrvc     services.IAggregationService
	languageMappingSrvc services.ILanguageMappingService
	projectLabelSrvc    services.IProjectLabelService
	keyValueSrvc        services.IKeyValueService
	mailSrvc            services.IMailService
	httpClient          *http.Client
	aggregationLocks    map[string]bool
}

type action func(w http.ResponseWriter, r *http.Request) actionResult

type actionResult struct {
	code    int
	success string
	error   string
	values  *map[string]interface{}
}

const valueInviteCode = "invite_code"

var credentialsDecoder = schema.NewDecoder()

func NewSettingsHandler(
	userService services.IUserService,
	heartbeatService services.IHeartbeatService,
	durationService services.IDurationService,
	summaryService services.ISummaryService,
	aliasService services.IAliasService,
	aggregationService services.IAggregationService,
	languageMappingService services.ILanguageMappingService,
	projectLabelService services.IProjectLabelService,
	keyValueService services.IKeyValueService,
	mailService services.IMailService,
) *SettingsHandler {
	return &SettingsHandler{
		config:              conf.Get(),
		summarySrvc:         summaryService,
		aliasSrvc:           aliasService,
		aggregationSrvc:     aggregationService,
		languageMappingSrvc: languageMappingService,
		projectLabelSrvc:    projectLabelService,
		userSrvc:            userService,
		heartbeatSrvc:       heartbeatService,
		durationSrvc:        durationService,
		keyValueSrvc:        keyValueService,
		mailSrvc:            mailService,
		httpClient:          &http.Client{Timeout: 10 * time.Second},
		aggregationLocks:    make(map[string]bool),
	}
}

func (h *SettingsHandler) RegisterRoutes(router chi.Router) {
	r := chi.NewRouter()
	r.Use(
		middlewares.NewAuthenticateMiddleware(h.userSrvc).
			WithRedirectTarget(defaultErrorRedirectTarget()).
			WithRedirectErrorMessage("unauthorized").Handler,
	)
	r.Get("/", h.GetIndex)
	r.Post("/", h.PostIndex)

	router.Mount("/settings", r)
}

func (h *SettingsHandler) GetIndex(w http.ResponseWriter, r *http.Request) {
	if h.config.IsDev() {
		loadTemplates()
	}
	templates[conf.SettingsTemplate].Execute(w, h.buildViewModel(r, w, nil))
}

func (h *SettingsHandler) PostIndex(w http.ResponseWriter, r *http.Request) {
	if h.config.IsDev() {
		loadTemplates()
	}

	if err := r.ParseForm(); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		templates[conf.SettingsTemplate].Execute(w, h.buildViewModel(r, w, nil).WithError("missing form values"))
		return
	}

	action := r.PostForm.Get("action")
	r.PostForm.Del("action")

	actionFunc := h.dispatchAction(action)
	if actionFunc == nil {
		slog.Warn("failed to dispatch action", "action", action)
		w.WriteHeader(http.StatusBadRequest)
		templates[conf.SettingsTemplate].Execute(w, h.buildViewModel(r, w, nil).WithError("未知操作请求"))
		return
	}

	result := actionFunc(w, r)

	// action responded itself
	if result.code == -1 {
		return
	}

	if result.error != "" {
		w.WriteHeader(result.code)
		templates[conf.SettingsTemplate].Execute(w, h.buildViewModel(r, w, result.values).WithError(result.error))
		return
	}
	if result.success != "" {
		w.WriteHeader(result.code)
		templates[conf.SettingsTemplate].Execute(w, h.buildViewModel(r, w, result.values).WithSuccess(result.success))
		return
	}
	templates[conf.SettingsTemplate].Execute(w, h.buildViewModel(r, w, result.values))
}

func (h *SettingsHandler) dispatchAction(action string) action {
	switch action {
	case "change_password":
		return h.actionChangePassword
	case "change_userid":
		return h.actionChangeUserId
	case "update_user":
		return h.actionUpdateUser
	case "reset_apikey":
		return h.actionResetApiKey
	case "delete_alias":
		return h.actionDeleteAlias
	case "add_alias":
		return h.actionAddAlias
	case "add_label":
		return h.actionAddLabel
	case "delete_label":
		return h.actionDeleteLabel
	case "delete_mapping":
		return h.actionDeleteLanguageMapping
	case "add_mapping":
		return h.actionAddLanguageMapping
	case "update_sharing":
		return h.actionUpdateSharing
	case "update_leaderboard":
		return h.actionUpdateLeaderboard
	case "toggle_wakatime":
		return h.actionSetWakatimeApiKey
	case "import_wakatime":
		return h.actionImportWakatime
	case "regenerate_summaries":
		return h.actionRegenerateSummaries
	case "clear_data":
		return h.actionClearData
	case "delete_account":
		return h.actionDeleteUser
	case "generate_invite":
		return h.actionGenerateInvite
	case "update_unknown_projects":
		return h.actionUpdateExcludeUnknownProjects
	case "update_heartbeats_timeout":
		return h.actionUpdateHeartbeatsTimeout
	case "update_new_summary":
		return h.actionUpdateNewSummary
	}
	return nil
}

func (h *SettingsHandler) actionUpdateUser(w http.ResponseWriter, r *http.Request) actionResult {
	if h.config.IsDev() {
		loadTemplates()
	}

	user := middlewares.GetPrincipal(r)

	var payload models.UserDataUpdate
	if err := r.ParseForm(); err != nil {
		return actionResult{http.StatusBadRequest, "", "缺少必需参数", nil}
	}
	if err := credentialsDecoder.Decode(&payload, r.PostForm); err != nil {
		return actionResult{http.StatusBadRequest, "", "缺少必需参数", nil}
	}

	if !payload.IsValid() {
		return actionResult{http.StatusBadRequest, "", "参数无效 - 可能是电子邮件地址格式错误？", nil}
	}

	if payload.Email == "" && user.HasActiveSubscription() {
		return actionResult{http.StatusBadRequest, "", "订阅处于活跃状态时无法取消设置电子邮件", nil}
	}

	user.Email = payload.Email
	user.Location = payload.Location
	user.StartOfWeek = payload.StartOfWeek
	user.ReportsWeekly = payload.ReportsWeekly
	user.PublicLeaderboard = payload.PublicLeaderboard

	if _, err := h.userSrvc.Update(user); err != nil {
		if strings.Contains(err.Error(), "email address already in use") {
			return actionResult{http.StatusBadRequest, "", "用户数据无效（电子邮件地址可能已被占用？）", nil}
		}
		return actionResult{http.StatusInternalServerError, "", conf.ErrInternalServerError, nil}
	}

	return actionResult{http.StatusOK, "用户更新成功", "", nil}
}

func (h *SettingsHandler) actionChangePassword(w http.ResponseWriter, r *http.Request) actionResult {
	if h.config.IsDev() {
		loadTemplates()
	}

	user := middlewares.GetPrincipal(r)

	if user.AuthType != "local" {
		return actionResult{http.StatusBadRequest, "", "无法为非本地用户重置密码", nil}
	}

	var credentials models.CredentialsReset
	if err := r.ParseForm(); err != nil {
		return actionResult{http.StatusBadRequest, "", "缺少必需参数", nil}
	}
	if err := credentialsDecoder.Decode(&credentials, r.PostForm); err != nil {
		return actionResult{http.StatusBadRequest, "", "缺少必需参数", nil}
	}

	if !utils.ComparePassword(user.Password, credentials.PasswordOld, h.config.Security.PasswordSalt) {
		return actionResult{http.StatusUnauthorized, "", "凭据无效", nil}
	}

	if !credentials.IsValid() {
		return actionResult{http.StatusBadRequest, "", "参数无效", nil}
	}

	user.Password = credentials.PasswordNew
	if hash, err := utils.HashPassword(user.Password, h.config.Security.PasswordSalt); err != nil {
		return actionResult{http.StatusInternalServerError, "", conf.ErrInternalServerError, nil}
	} else {
		user.Password = hash
	}

	if _, err := h.userSrvc.Update(user); err != nil {
		return actionResult{http.StatusInternalServerError, "", conf.ErrInternalServerError, nil}
	}

	login := &models.Login{
		Username: user.ID,
		Password: user.Password,
	}
	encoded, err := h.config.Security.SecureCookie.Encode(models.AuthCookieKey, login.Username)
	if err != nil {
		return actionResult{http.StatusInternalServerError, "", conf.ErrInternalServerError, nil}
	}

	http.SetCookie(w, h.config.CreateCookie(models.AuthCookieKey, encoded))
	return actionResult{http.StatusOK, "密码更新成功", "", nil}
}

func (h *SettingsHandler) actionChangeUserId(w http.ResponseWriter, r *http.Request) actionResult {
	if h.config.IsDev() {
		loadTemplates()
	}

	user := middlewares.GetPrincipal(r)

	newUserId := strings.TrimSpace(r.PostFormValue("new_userid"))
	if !models.ValidateUsername(newUserId) || newUserId == user.ID {
		return actionResult{http.StatusBadRequest, "", "用户名无效", nil}
	}
	if existing, _ := h.userSrvc.GetUserById(newUserId); existing != nil {
		return actionResult{http.StatusConflict, "", "已被占用", nil}
	}

	if _, err := h.userSrvc.ChangeUserId(user, newUserId); err != nil {
		return actionResult{http.StatusInternalServerError, "", conf.ErrInternalServerError, nil}
	}

	routeutils.SetSuccess(r, w, fmt.Sprintf("成功将您的用户名更改为 %s，请重新登录。", newUserId))
	http.SetCookie(w, h.config.GetClearCookie(models.AuthCookieKey))
	http.Redirect(w, r, h.config.Server.BasePath, http.StatusFound)
	return actionResult{-1, "", "", nil}
}

func (h *SettingsHandler) actionResetApiKey(w http.ResponseWriter, r *http.Request) actionResult {
	if h.config.IsDev() {
		loadTemplates()
	}

	user := middlewares.GetPrincipal(r)
	if _, err := h.userSrvc.ResetApiKey(user); err != nil {
		return actionResult{http.StatusInternalServerError, "", conf.ErrInternalServerError, nil}
	}

	msg := fmt.Sprintf("您的新 API 密钥为：%s", user.ApiKey)
	return actionResult{http.StatusOK, msg, "", nil}
}

func (h *SettingsHandler) actionUpdateLeaderboard(w http.ResponseWriter, r *http.Request) actionResult {
	if h.config.IsDev() {
		loadTemplates()
	}

	var err error
	user := middlewares.GetPrincipal(r)
	defer h.userSrvc.FlushCache()

	user.PublicLeaderboard, err = strconv.ParseBool(r.PostFormValue("enable_leaderboard"))

	if err != nil {
		return actionResult{http.StatusBadRequest, "", "输入无效", nil}
	}
	if _, err := h.userSrvc.Update(user); err != nil {
		return actionResult{http.StatusInternalServerError, "", "服务器内部错误", nil}
	}
	return actionResult{http.StatusOK, "设置已更新", "", nil}
}

func (h *SettingsHandler) actionUpdateExcludeUnknownProjects(w http.ResponseWriter, r *http.Request) actionResult {
	if h.config.IsDev() {
		loadTemplates()
	}

	var err error
	user := middlewares.GetPrincipal(r)
	defer h.userSrvc.FlushCache()

	if h.isAggregationLocked(user.ID) {
		return actionResult{http.StatusConflict, "", "摘要重新生成正在进行中，请稍候", nil}
	}

	user.ExcludeUnknownProjects, err = strconv.ParseBool(r.PostFormValue("exclude_unknown_projects"))

	if err != nil {
		return actionResult{http.StatusBadRequest, "", "输入无效", nil}
	}
	if _, err := h.userSrvc.Update(user); err != nil {
		return actionResult{http.StatusInternalServerError, "", "服务器内部错误", nil}
	}

	go func(user *models.User, r *http.Request) {
		h.toggleAggregationLock(user.ID, true)
		defer h.toggleAggregationLock(user.ID, false)
		if err := h.regenerateSummaries(user); err != nil {
			conf.Log().Request(r).Error("failed to regenerate summaries for user", "userID", user.ID, "error", err)
		}
	}(user, r)

	return actionResult{http.StatusOK, "正在重新生成摘要，这可能需要一段时间", "", nil}
}

func (h *SettingsHandler) actionUpdateHeartbeatsTimeout(w http.ResponseWriter, r *http.Request) actionResult {
	if h.config.IsDev() {
		loadTemplates()
	}

	var err error
	user := middlewares.GetPrincipal(r)
	defer h.userSrvc.FlushCache()

	val, err := strconv.ParseInt(r.PostFormValue("heartbeats_timeout"), 0, 0)
	dur := time.Duration(val) * time.Minute
	if err != nil || dur < models.MinHeartbeatsTimeout || dur > models.MaxHeartbeatsTimeout {
		return actionResult{http.StatusBadRequest, "", "输入无效", nil}
	}
	user.HeartbeatsTimeoutSec = int(dur.Seconds())

	if _, err := h.userSrvc.Update(user); err != nil {
		return actionResult{http.StatusInternalServerError, "", "服务器内部错误", nil}
	}

	return actionResult{http.StatusOK, "完成。要将此更改应用于现有数据，请重新生成您的摘要。", "", nil}
}

func (h *SettingsHandler) actionUpdateNewSummary(w http.ResponseWriter, r *http.Request) actionResult {
	if h.config.IsDev() {
		loadTemplates()
	}

	user := middlewares.GetPrincipal(r)
	defer h.userSrvc.FlushCache()

	val, err := strconv.ParseBool(r.PostFormValue("new_summary"))
	if err != nil {
		return actionResult{http.StatusBadRequest, "", "输入无效", nil}
	}

	user.NewSummary = val
	if _, err := h.userSrvc.Update(user); err != nil {
		return actionResult{http.StatusInternalServerError, "", "服务器内部错误", nil}
	}

	return actionResult{http.StatusOK, "摘要界面设置已更新", "", nil}
}

func (h *SettingsHandler) actionUpdateSharing(w http.ResponseWriter, r *http.Request) actionResult {
	if h.config.IsDev() {
		loadTemplates()
	}

	var err error
	user := middlewares.GetPrincipal(r)

	defer h.userSrvc.FlushUserCache(user.ID)

	user.ShareProjects, err = strconv.ParseBool(r.PostFormValue("share_projects"))
	user.ShareLanguages, err = strconv.ParseBool(r.PostFormValue("share_languages"))
	user.ShareEditors, err = strconv.ParseBool(r.PostFormValue("share_editors"))
	user.ShareOSs, err = strconv.ParseBool(r.PostFormValue("share_oss"))
	user.ShareMachines, err = strconv.ParseBool(r.PostFormValue("share_machines"))
	user.ShareLabels, err = strconv.ParseBool(r.PostFormValue("share_labels"))
	user.ShareActivityChart, err = strconv.ParseBool(r.PostFormValue("share_activity_chart"))
	user.ShareDataMaxDays, err = strconv.Atoi(r.PostFormValue("max_days"))

	if err != nil {
		return actionResult{http.StatusBadRequest, "", "输入无效", nil}
	}

	if _, err := h.userSrvc.Update(user); err != nil {
		return actionResult{http.StatusInternalServerError, "", "服务器内部错误", nil}
	}

	return actionResult{http.StatusOK, "设置已更新", "", nil}
}

func (h *SettingsHandler) actionDeleteAlias(w http.ResponseWriter, r *http.Request) actionResult {
	if h.config.IsDev() {
		loadTemplates()
	}

	user := middlewares.GetPrincipal(r)
	aliasKey := r.PostFormValue("key")
	aliasType, err := strconv.Atoi(r.PostFormValue("type"))
	if err != nil {
		aliasType = 99 // nothing will be found later on
	}

	if aliases, err := h.aliasSrvc.GetByUserAndKeyAndType(user.ID, aliasKey, uint8(aliasType)); err != nil {
		return actionResult{http.StatusNotFound, "", "未找到别名", nil}
	} else if err := h.aliasSrvc.DeleteMulti(aliases); err != nil {
		return actionResult{http.StatusInternalServerError, "", "无法删除别名", nil}
	}

	return actionResult{http.StatusOK, "别名删除成功", "", nil}
}

func (h *SettingsHandler) actionAddAlias(w http.ResponseWriter, r *http.Request) actionResult {
	if h.config.IsDev() {
		loadTemplates()
	}
	user := middlewares.GetPrincipal(r)
	aliasKey := r.PostFormValue("key")
	aliasValue := r.PostFormValue("value")
	aliasType, err := strconv.Atoi(r.PostFormValue("type"))
	if err != nil {
		aliasType = 99 // Alias.IsValid() will return false later on
	}

	alias := &models.Alias{
		UserID: user.ID,
		Key:    aliasKey,
		Value:  aliasValue,
		Type:   uint8(aliasType),
	}

	if _, err := h.aliasSrvc.Create(alias); err != nil {
		// TODO: distinguish between bad request, conflict and server error
		return actionResult{http.StatusBadRequest, "", "输入无效", nil}
	}

	return actionResult{http.StatusOK, "别名添加成功", "", nil}
}

func (h *SettingsHandler) actionAddLabel(w http.ResponseWriter, r *http.Request) actionResult {
	if h.config.IsDev() {
		loadTemplates()
	}
	user := middlewares.GetPrincipal(r)

	var labels []*models.ProjectLabel

	for _, key := range r.Form["key"] {
		label := &models.ProjectLabel{
			UserID:     user.ID,
			ProjectKey: key,
			Label:      r.PostFormValue("value"),
		}
		labels = append(labels, label)
	}

	for _, label := range labels {
		msg := "项目输入无效：" + label.ProjectKey
		if !label.IsValid() {
			return actionResult{http.StatusBadRequest, "", msg, nil}
		}
		if _, err := h.projectLabelSrvc.Create(label); err != nil {
			// TODO: distinguish between bad request, conflict and server error
			return actionResult{http.StatusBadRequest, "", msg, nil}
		}
	}
	return actionResult{http.StatusOK, "标签添加到项目成功", "", nil}
}

func (h *SettingsHandler) actionDeleteLabel(w http.ResponseWriter, r *http.Request) actionResult {
	if h.config.IsDev() {
		loadTemplates()
	}

	user := middlewares.GetPrincipal(r)
	labelKey := r.PostFormValue("key")     // label key
	labelValue := r.PostFormValue("value") // project key

	labels, err := h.projectLabelSrvc.GetByUser(user.ID)
	if err != nil {
		return actionResult{http.StatusInternalServerError, "", "无法删除标签", nil}
	}

	for _, l := range labels {
		if l.Label == labelKey && l.ProjectKey == labelValue {
			if err := h.projectLabelSrvc.Delete(l); err != nil {
				return actionResult{http.StatusInternalServerError, "", "无法删除标签", nil}
			}
			return actionResult{http.StatusOK, "标签删除成功", "", nil}
		}
	}
	return actionResult{http.StatusNotFound, "", "未找到标签", nil}
}

func (h *SettingsHandler) actionDeleteLanguageMapping(w http.ResponseWriter, r *http.Request) actionResult {
	if h.config.IsDev() {
		loadTemplates()
	}

	user := middlewares.GetPrincipal(r)
	id, err := strconv.Atoi(r.PostFormValue("mapping_id"))
	if err != nil {
		return actionResult{http.StatusInternalServerError, "", "无法删除映射", nil}
	}

	mapping, err := h.languageMappingSrvc.GetById(uint(id))
	if err != nil || mapping == nil {
		return actionResult{http.StatusNotFound, "", "未找到映射", nil}
	} else if mapping.UserID != user.ID {
		return actionResult{http.StatusForbidden, "", "不允许删除此映射", nil}
	}

	if err := h.languageMappingSrvc.Delete(mapping); err != nil {
		return actionResult{http.StatusInternalServerError, "", "无法删除映射", nil}
	}

	return actionResult{http.StatusOK, "映射删除成功", "", nil}
}

func (h *SettingsHandler) actionAddLanguageMapping(w http.ResponseWriter, r *http.Request) actionResult {
	if h.config.IsDev() {
		loadTemplates()
	}
	user := middlewares.GetPrincipal(r)
	extension := r.PostFormValue("extension")
	language := r.PostFormValue("language")

	if extension[0] == '.' {
		extension = extension[1:]
	}

	mapping := &models.LanguageMapping{
		UserID:    user.ID,
		Extension: extension,
		Language:  language,
	}

	if _, err := h.languageMappingSrvc.Create(mapping); err != nil {
		return actionResult{http.StatusConflict, "", "映射已存在", nil}
	}

	return actionResult{http.StatusOK, "映射添加成功", "", nil}
}

func (h *SettingsHandler) actionSetWakatimeApiKey(w http.ResponseWriter, r *http.Request) actionResult {
	if h.config.IsDev() {
		loadTemplates()
	}

	user := middlewares.GetPrincipal(r)
	apiKey := r.PostFormValue("api_key")
	apiUrl := r.PostFormValue("api_url")
	if apiUrl == conf.WakatimeApiUrl || apiKey == "" {
		apiUrl = ""
	}

	// Healthcheck, if a new API key is set, i.e. the feature is activated
	if (user.WakatimeApiKey == "" && apiKey != "") && !h.validateWakatimeKey(apiKey, apiUrl) {
		return actionResult{http.StatusBadRequest, "", "连接 WakaTime 失败，API 密钥或端点 URL 无效？", nil}
	}

	if _, err := h.userSrvc.SetWakatimeApiCredentials(user, apiKey, apiUrl); err != nil {
		return actionResult{http.StatusInternalServerError, "", conf.ErrInternalServerError, nil}
	}

	return actionResult{http.StatusOK, "WakaTime API 密钥更新成功", "", nil}
}

func (h *SettingsHandler) actionImportWakatime(w http.ResponseWriter, r *http.Request) actionResult {
	if h.config.IsDev() {
		loadTemplates()
	}

	if !h.config.App.ImportEnabled {
		return actionResult{http.StatusForbidden, "", "此服务器已禁用导入功能", nil}
	}

	user := middlewares.GetPrincipal(r)
	if user.WakatimeApiKey == "" {
		return actionResult{http.StatusForbidden, "", "未连接到 WakaTime", nil}
	}

	useLegacyImporter, _ := strconv.ParseBool(r.PostFormValue("use_legacy_importer"))
	kvKeyLastImport := fmt.Sprintf("%s_%s", conf.KeyLastImport, user.ID)
	kvKeyLastImportSuccess := fmt.Sprintf("%s_%s", conf.KeyLastImportSuccess, user.ID)

	if !h.config.IsDev() {
		lastImport, _ := time.Parse(time.RFC822, h.keyValueSrvc.MustGetString(kvKeyLastImport).Value)
		if time.Now().Sub(lastImport) < time.Duration(h.config.App.ImportBackoffMin)*time.Minute {
			return actionResult{
				http.StatusTooManyRequests,
				"",
				fmt.Sprintf("数据导入过于频繁 - 您仅允许每 %d 分钟请求一次导入。", h.config.App.ImportBackoffMin),
				nil,
			}
		}

		lastImportSuccess, _ := time.Parse(time.RFC822, h.keyValueSrvc.MustGetString(kvKeyLastImportSuccess).Value)
		if time.Now().Sub(lastImportSuccess) < time.Duration(h.config.App.ImportMaxRate)*time.Hour {
			return actionResult{
				http.StatusTooManyRequests,
				"",
				fmt.Sprintf("数据导入过于频繁 - 上次导入在 %d 小时前运行，请稍候。", h.config.App.ImportMaxRate),
				nil,
			}
		}
	}

	go func(user *models.User, r *http.Request) {
		start := time.Now()
		importer := imports.NewWakatimeImporter(user.WakatimeApiKey, useLegacyImporter)

		countBefore, _ := h.heartbeatSrvc.CountByUser(user)

		var (
			stream      <-chan *models.Heartbeat
			importError error
		)
		if latest, err := h.heartbeatSrvc.GetLatestByOriginAndUser(imports.OriginWakatime, user); latest == nil || err != nil {
			stream, importError = importer.ImportAll(user)
		} else {
			// if an import has happened before, only import heartbeats newer than the latest of the last import
			stream, importError = importer.Import(user, latest.Time.T(), time.Now())
		}
		if importError != nil {
			conf.Log().Error("wakatime import for user failed", "userID", user.ID, "error", importError)
			return
		}

		// import successful
		h.keyValueSrvc.PutString(&models.KeyStringValue{
			Key:   kvKeyLastImportSuccess,
			Value: time.Now().Format(time.RFC822),
		})

		count := 0
		batch := make([]*models.Heartbeat, 0, h.config.App.ImportBatchSize)

		insert := func(batch []*models.Heartbeat) {
			if err := h.heartbeatSrvc.InsertBatch(batch); err != nil {
				slog.Warn("failed to insert imported heartbeat, already existing?", "error", err)
			}
		}

		for hb := range stream {
			count++
			batch = append(batch, hb)

			if len(batch) == h.config.App.ImportBatchSize {
				insert(batch)
				batch = make([]*models.Heartbeat, 0, h.config.App.ImportBatchSize)
			}
		}
		if len(batch) > 0 {
			insert(batch)
		}

		countAfter, _ := h.heartbeatSrvc.CountByUser(user)
		slog.Info("downloaded heartbeats for user", "count", count, "userID", user.ID, "importedCount", countAfter-countBefore)

		h.regenerateSummaries(user)

		if !user.HasData {
			user.HasData = true
			if _, err := h.userSrvc.Update(user); err != nil {
				conf.Log().Request(r).Error("failed to set 'has_data' flag for user", "userID", user.ID, "error", err)
			}
		}

		if user.Email != "" {
			if err := h.mailSrvc.SendImportNotification(user, time.Now().Sub(start), int(countAfter-countBefore)); err != nil {
				conf.Log().Request(r).Error("failed to send import notification mail", "userID", user.ID, "error", err)
			} else {
				slog.Info("sent import notification mail", "userID", user.ID)
			}
		}
	}(user, r)

	h.keyValueSrvc.PutString(&models.KeyStringValue{
		Key:   kvKeyLastImport,
		Value: time.Now().Format(time.RFC822),
	})

	return actionResult{http.StatusAccepted, "导入已开始。这将需要几分钟时间，请稍后查看。", "", nil}
}

func (h *SettingsHandler) actionRegenerateSummaries(w http.ResponseWriter, r *http.Request) actionResult {
	if h.config.IsDev() {
		loadTemplates()
	}

	user := middlewares.GetPrincipal(r)

	if h.isAggregationLocked(user.ID) {
		return actionResult{http.StatusConflict, "", "摘要重新生成正在进行中，请稍候", nil}
	}

	go func(user *models.User, r *http.Request) {
		h.toggleAggregationLock(user.ID, true)
		defer h.toggleAggregationLock(user.ID, false)
		if err := h.regenerateSummaries(user); err != nil {
			conf.Log().Request(r).Error("failed to regenerate summaries for user", "userID", user.ID, "error", err)
		}
	}(user, r)

	return actionResult{http.StatusAccepted, "摘要正在重新生成 - 这可能需要几分钟时间，请稍后再来", "", nil}
}

func (h *SettingsHandler) actionClearData(w http.ResponseWriter, r *http.Request) actionResult {
	if h.config.IsDev() {
		loadTemplates()
	}

	user := middlewares.GetPrincipal(r)
	slog.Info("user requested to delete all data", "userID", user.ID)

	go func(user *models.User, r *http.Request) {
		slog.Info("deleting summaries for user", "userID", user.ID)
		if err := h.summarySrvc.DeleteByUser(user.ID); err != nil {
			conf.Log().Request(r).Error("failed to clear summaries", "error", err)
		}

		slog.Info("deleting durations for user", "userID", user.ID)
		if err := h.durationSrvc.DeleteByUser(user); err != nil {
			conf.Log().Request(r).Error("failed to clear durations", "error", err)
		}

		slog.Info("deleting heartbeats for user", "userID", user.ID)
		if err := h.heartbeatSrvc.DeleteByUser(user); err != nil {
			conf.Log().Request(r).Error("failed to clear heartbeats", "error", err)
		}
	}(user, r)

	return actionResult{http.StatusAccepted, "删除进行中，这可能需要几秒钟", "", nil}
}

func (h *SettingsHandler) actionDeleteUser(w http.ResponseWriter, r *http.Request) actionResult {
	if h.config.IsDev() {
		loadTemplates()
	}

	user := middlewares.GetPrincipal(r)
	go func(user *models.User, r *http.Request) {
		slog.Info("deleting user shortly", "userID", user.ID)
		//time.Sleep(5 * time.Minute)
		if err := h.userSrvc.Delete(user); err != nil {
			conf.Log().Request(r).Error("failed to delete user", "userID", user.ID, "error", err)
		} else {
			slog.Info("successfully deleted user", "userID", user.ID)
		}
	}(user, r)

	routeutils.SetSuccess(r, w, "您的账户将在几分钟内删除。很遗憾看到您离开。")
	http.SetCookie(w, h.config.GetClearCookie(models.AuthCookieKey))
	http.Redirect(w, r, h.config.Server.BasePath, http.StatusFound)
	return actionResult{-1, "", "", nil}
}

func (h *SettingsHandler) actionGenerateInvite(w http.ResponseWriter, r *http.Request) actionResult {
	if h.config.IsDev() {
		loadTemplates()
	}

	user := middlewares.GetPrincipal(r)
	inviteCode := uuid.Must(uuid.NewV4()).String()[0:8]

	if err := h.keyValueSrvc.PutString(&models.KeyStringValue{
		Key:   fmt.Sprintf("%s_%s", conf.KeyInviteCode, inviteCode),
		Value: fmt.Sprintf("%s,%s", user.ID, time.Now().Format(time.RFC3339)),
	}); err != nil {
		return actionResult{http.StatusInternalServerError, "", "邀请码生成失败", nil}
	}

	return actionResult{
		http.StatusOK,
		"邀请码生成成功（见下方）",
		"",
		&map[string]interface{}{
			valueInviteCode: inviteCode,
		},
	}
}

func (h *SettingsHandler) validateWakatimeKey(apiKey string, baseUrl string) bool {
	if baseUrl == "" {
		baseUrl = conf.WakatimeApiUrl
	}

	headers := http.Header{
		"Accept": []string{"application/json"},
		"Authorization": []string{
			fmt.Sprintf("Basic %s", base64.StdEncoding.EncodeToString([]byte(apiKey))),
		},
	}

	request, err := http.NewRequest(
		http.MethodGet,
		baseUrl+conf.WakatimeApiUserUrl,
		nil,
	)
	if err != nil {
		return false
	}

	request.Header = headers

	if _, err = utils.RaiseForStatus(h.httpClient.Do(request)); err != nil {
		return false
	}

	return true
}

func (h *SettingsHandler) regenerateSummaries(user *models.User) error {
	slog.Info("clearing summaries and durations for user", "userID", user.ID)

	if err := h.summarySrvc.DeleteByUser(user.ID); err != nil {
		conf.Log().Error("failed to clear summaries", "error", err)
		return err
	}

	if err := h.aggregationSrvc.AggregateSummaries(datastructure.New(user.ID)); err != nil { // involves regenerating durations as well
		conf.Log().Error("failed to regenerate summaries", "error", err)
		return err
	}

	return nil
}

func (h *SettingsHandler) buildViewModel(r *http.Request, w http.ResponseWriter, args *map[string]interface{}) *view.SettingsViewModel {
	user := middlewares.GetPrincipal(r)

	// mappings
	mappings, _ := h.languageMappingSrvc.GetByUser(user.ID)

	// aliases
	aliases, err := h.aliasSrvc.GetByUser(user.ID)
	if err != nil {
		conf.Log().Request(r).Error("error while building alias map", "error", err)
		return &view.SettingsViewModel{
			SharedLoggedInViewModel: view.SharedLoggedInViewModel{
				SharedViewModel: view.NewSharedViewModel(h.config, &view.Messages{Error: criticalError}),
				User:            user,
			},
		}
	}
	aliasMap := make(map[string][]*models.Alias)
	for _, a := range aliases {
		k := fmt.Sprintf("%s_%d", a.Key, a.Type)
		if _, ok := aliasMap[k]; !ok {
			aliasMap[k] = []*models.Alias{a}
		} else {
			aliasMap[k] = append(aliasMap[k], a)
		}
	}

	combinedAliases := make([]*view.SettingsVMCombinedAlias, 0)
	for _, l := range aliasMap {
		ca := &view.SettingsVMCombinedAlias{
			Key:    l[0].Key,
			Type:   l[0].Type,
			Values: make([]string, len(l)),
		}
		for i, a := range l {
			ca.Values[i] = a.Value
		}
		combinedAliases = append(combinedAliases, ca)
	}

	// labels
	labelMap, err := h.projectLabelSrvc.GetByUserGroupedInverted(user.ID)
	if err != nil {
		conf.Log().Request(r).Error("error while building settings project label map", "error", err)
		return &view.SettingsViewModel{
			SharedLoggedInViewModel: view.SharedLoggedInViewModel{
				SharedViewModel: view.NewSharedViewModel(h.config, &view.Messages{Error: criticalError}),
				User:            user,
			},
		}
	}

	combinedLabels := make([]*view.SettingsVMCombinedLabel, 0)
	for _, l := range labelMap {
		cl := &view.SettingsVMCombinedLabel{
			Key:    l[0].Label,
			Values: make([]string, len(l)),
		}
		for i, l1 := range l {
			cl.Values[i] = l1.ProjectKey
		}
		combinedLabels = append(combinedLabels, cl)
	}
	sort.Slice(combinedLabels, func(i, j int) bool {
		return strings.Compare(combinedLabels[i].Key, combinedLabels[j].Key) < 0
	})

	// projects
	projects, err := routeutils.GetEffectiveProjectsList(user, h.heartbeatSrvc, h.aliasSrvc)
	if err != nil {
		conf.Log().Request(r).Error("error while fetching projects", "error", err)
		return &view.SettingsViewModel{
			SharedLoggedInViewModel: view.SharedLoggedInViewModel{
				SharedViewModel: view.NewSharedViewModel(h.config, &view.Messages{Error: criticalError}),
				User:            user,
			},
		}
	}

	// subscriptions
	var subscriptionPrice string
	if h.config.Subscriptions.Enabled {
		subscriptionPrice = h.config.Subscriptions.StandardPrice
	}

	// user first data
	firstData, err := h.heartbeatSrvc.GetFirstByUser(user)
	if err != nil {
		conf.Log().Request(r).Error("error while user's heartbeats range", "user", user.ID, "error", err)
		return &view.SettingsViewModel{
			SharedLoggedInViewModel: view.SharedLoggedInViewModel{
				SharedViewModel: view.NewSharedViewModel(h.config, &view.Messages{Error: criticalError}),
				User:            user,
			},
		}
	}

	// invite link
	inviteCode := getVal[string](args, valueInviteCode, "")
	inviteLink := condition.Ternary[bool, string](inviteCode == "", "", fmt.Sprintf("%s/signup?invite=%s", h.config.Server.GetPublicUrl(), inviteCode))

	vm := &view.SettingsViewModel{
		SharedLoggedInViewModel: view.SharedLoggedInViewModel{
			SharedViewModel: view.NewSharedViewModel(h.config, nil),
			User:            user,
		},
		LanguageMappings:    mappings,
		Aliases:             combinedAliases,
		Labels:              combinedLabels,
		Projects:            projects,
		UserFirstData:       firstData,
		SubscriptionPrice:   subscriptionPrice,
		SupportContact:      h.config.App.SupportContact,
		DataRetentionMonths: h.config.App.DataRetentionMonths,
		InviteLink:          inviteLink,
	}

	// readme card params
	readmeCardTitle := "Wakapi.dev Stats"
	if err, maxRange := helpers.ResolveMaximumRange(user.ShareDataMaxDays); err == nil {
		readmeCardTitle += fmt.Sprintf(" (%v)", maxRange.GetHumanReadable())
	}
	vm.ReadmeCardCustomTitle = url.QueryEscape(readmeCardTitle)

	return routeutils.WithSessionMessages(vm, r, w)
}

func (h *SettingsHandler) toggleAggregationLock(userId string, locked bool) {
	h.aggregationLocks[userId] = locked
}

func (h *SettingsHandler) isAggregationLocked(userId string) bool {
	locked, _ := h.aggregationLocks[userId]
	return locked
}

func getVal[T any](values *map[string]interface{}, key string, fallback T) T {
	if values == nil {
		return fallback
	}
	valuesMap := *values
	val, ok := valuesMap[key]
	if !ok {
		return fallback
	}
	return val.(T)
}

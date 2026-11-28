package handler

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/kraconst/GO_server/internal/domain"
	"github.com/kraconst/GO_server/internal/usecase"
)

type Handler struct {
	accountUsecase           *usecase.AccountUsecase
	employeeUsecase          *usecase.EmployeeUsecase
	messageUsecase           *usecase.MessageUsecase
	contactUsecase           *usecase.ContactUsecase
	chatUsecase              *usecase.ChatUsecase
	accountDailyStatsUsecase *usecase.AccountDailyStatsUsecase
	alertUsecase             *usecase.AlertUsecase
	companyUsecase           *usecase.CompanyUsecase
	summaryUsecase           *usecase.ChatSummaryUsecase
	userUsecase              *usecase.UserUsecase
	accountSync              AccountSyncService
	demoAlerts               *demoAlertStore
}

func New(
	accountUsecase *usecase.AccountUsecase,
	employeeUsecase *usecase.EmployeeUsecase,
	messageUsecase *usecase.MessageUsecase,
	contactUsecase *usecase.ContactUsecase,
	chatUsecase *usecase.ChatUsecase,
	accountDailyStatsUsecase *usecase.AccountDailyStatsUsecase,
	alertUsecase *usecase.AlertUsecase,
	companyUsecase *usecase.CompanyUsecase,
	summaryUsecase *usecase.ChatSummaryUsecase,
	userUsecase *usecase.UserUsecase,
) *Handler {
	return &Handler{
		accountUsecase:           accountUsecase,
		employeeUsecase:          employeeUsecase,
		messageUsecase:           messageUsecase,
		contactUsecase:           contactUsecase,
		chatUsecase:              chatUsecase,
		accountDailyStatsUsecase: accountDailyStatsUsecase,
		alertUsecase:             alertUsecase,
		companyUsecase:           companyUsecase,
		summaryUsecase:           summaryUsecase,
		userUsecase:              userUsecase,
		demoAlerts:               newDemoAlertStore(),
	}
}

var mockMu sync.RWMutex

var mockAccounts = []domain.Account{
	{ID: 1, CompanyID: 1, EmployeeID: 1, Phone: "+79991112233", Username: "@sales_anna", EmployeeName: "Анна", Status: domain.StatusActive, CreatedAt: time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)},
	{ID: 2, CompanyID: 1, EmployeeID: 2, Phone: "+79994445566", Username: "@support_oleg", EmployeeName: "Олег", Status: domain.StatusDisconnected, CreatedAt: time.Date(2026, 9, 15, 14, 30, 0, 0, time.UTC)},
}

var mockEmployees = []domain.Employee{
	{ID: 1, CompanyID: 1, Name: "Анна", Position: "Менеджер по продажам", IsActive: true},
	{ID: 2, CompanyID: 1, Name: "Олег", Position: "Служба поддержки", IsActive: true},
}

var mockMessages = []domain.Message{
	{ID: 1, ChatID: 1, TelegramMessageID: 101, Direction: domain.MessageDirectionIn, Text: "Здравствуйте! Подскажите по условиям.", SentAt: time.Date(2026, 9, 1, 10, 5, 0, 0, time.UTC)},
	{ID: 2, ChatID: 1, TelegramMessageID: 102, Direction: domain.MessageDirectionOut, Text: "Добрый день! Сейчас направлю КП.", SentAt: time.Date(2026, 9, 1, 10, 6, 0, 0, time.UTC)},
}

var mockContacts = []domain.Contact{
	{
		ID:             1,
		CompanyID:      1,
		TelegramUserID: 10001,
		Name:           "Иван Петров",
		Username:       "ivan_p",
	},
	{
		ID:             2,
		CompanyID:      1,
		TelegramUserID: 10002,
		Name:           "Мария Смирнова",
		Username:       "maria_s",
	},
}

var mockChats = []domain.Chat{
	{
		ID:                 1,
		AccountID:          1,
		ContactID:          1,
		Status:             domain.ChatStatusNewLead,
		StatusSource:       domain.StatusSourceAuto,
		Priority:           domain.ChatPriorityNormal,
		LastMessageAt:      time.Date(2026, 9, 1, 10, 6, 0, 0, time.UTC),
		AwaitingReplySince: nil,
		UnreadCount:        0,
	},
	{
		ID:                 2,
		AccountID:          2,
		ContactID:          2,
		Status:             domain.ChatStatusNegotiation,
		StatusSource:       domain.StatusSourceManual,
		Priority:           domain.ChatPriorityHigh,
		LastMessageAt:      time.Date(2026, 9, 15, 15, 0, 0, 0, time.UTC),
		AwaitingReplySince: nil,
		UnreadCount:        1,
	},
}

var mockDashboard = domain.DashboardStats{
	MessagesIn:      1420,
	MessagesOut:     1180,
	ActiveChats:     85,
	NewContacts:     34,
	AvgResponseTime: 4 * time.Minute,
	UnansweredChats: 3,
}

var mockAccountRatings = []domain.AccountRating{
	{
		AccountID:       1,
		Username:        "@sales_anna",
		EmployeeName:    "Анна",
		MessagesIn:      850,
		MessagesOut:     720,
		TotalMessages:   1570,
		ActiveChats:     52,
		NewContacts:     21,
		AvgResponseTime: 3 * time.Minute,
	},
	{
		AccountID:       2,
		Username:        "@support_oleg",
		EmployeeName:    "Олег",
		MessagesIn:      570,
		MessagesOut:     460,
		TotalMessages:   1030,
		ActiveChats:     33,
		NewContacts:     13,
		AvgResponseTime: 5 * time.Minute,
	},
}

type createAccountRequest struct {
	CompanyID  int64  `json:"company_id"`
	EmployeeID int64  `json:"employee_id"`
	Phone      string `json:"phone" binding:"required"`
	Username   string `json:"username"`
}

type assignAccountEmployeeRequest struct {
	EmployeeID int64 `json:"employee_id" binding:"required"`
}

type createEmployeeRequest struct {
	CompanyID int64  `json:"company_id"`
	Name      string `json:"name" binding:"required"`
	Position  string `json:"position"`
}

type createCompanyRequest struct {
	Name             string `json:"name" binding:"required"`
	Timezone         string `json:"timezone"`
	NoReplyThreshold int64  `json:"no_reply_threshold"`
}

type createUserRequest struct {
	CompanyID    int64           `json:"company_id"`
	Email        string          `json:"email" binding:"required"`
	PasswordHash string          `json:"password_hash" binding:"required"`
	Role         domain.UserRole `json:"role"`
	EmployeeID   *int64          `json:"employee_id"`
}

type upsertChatSummaryRequest struct {
	Topic         string           `json:"topic"`
	Agreements    string           `json:"agreements"`
	NextStep      string           `json:"next_step"`
	OpenQuestions string           `json:"open_questions"`
	Risk          domain.RiskLevel `json:"risk"`
	LastMessageID int64            `json:"last_message_id"`
}

type updateEmployeeRequest struct {
	Name     string `json:"name" binding:"required"`
	Position string `json:"position"`
}

type updateChatStatusRequest struct {
	Status domain.ChatStatus `json:"status" binding:"required"`
}

const maxRequestBodyBytes int64 = 1 << 20 // 1 MiB

func (h *Handler) InitRoutes() *gin.Engine {
	r := gin.Default()
	r.Use(func(c *gin.Context) {
		if c.Request.Body != nil {
			c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxRequestBodyBytes)
		}
		c.Next()
	})
	r.GET("/health", h.Health)

	// Accounts
	r.GET("/accounts", h.List)
	r.GET("/accounts/:id", h.GetByID)
	r.POST("/accounts", h.Create)
	r.POST("/accounts/:id/assign", h.AssignEmployee)
	r.POST("/accounts/:id/sync", h.SyncAccount)

	// Employees
	r.GET("/employees", h.ListEmployees)
	r.GET("/employees/:id", h.GetEmployeeByID)
	r.POST("/employees", h.CreateEmployee)
	r.PUT("/employees/:id", h.UpdateEmployee)
	r.PATCH("/employees/:id", h.UpdateEmployee)
	r.POST("/employees/:id/deactivate", h.DeactivateEmployee)

	// Companies
	r.GET("/companies/:id", h.GetCompanyByID)
	r.POST("/companies", h.CreateCompany)

	// Users
	r.GET("/users", h.ListUsers)
	r.GET("/users/:id", h.GetUserByID)
	r.POST("/users", h.CreateUser)

	// Chats
	r.GET("/chats", h.ListChats)
	r.GET("/chats/:id", h.GetChatByID)
	r.POST("/chats/:id/status", h.UpdateChatStatus)

	// Chat Summaries
	r.GET("/chats/:id/summary", h.GetChatSummary)
	r.POST("/chats/:id/summary", h.UpsertChatSummary)
	r.POST("/chats/:id/summary/generate", h.GenerateChatSummary)

	// Contacts
	r.GET("/contacts", h.ListContacts)
	r.GET("/contacts/:id", h.GetContactByID)

	// Messages
	r.GET("/chats/:id/messages", h.ListMessagesByChat)
	r.GET("/chats/:id/analytics", h.GetChatAnalytics)

	// Analytics
	r.GET("/analytics/dashboard", h.GetDashboard)
	r.GET("/analytics/accounts", h.GetAccountRatings)

	// Alerts
	r.GET("/alerts", h.ListAlerts)
	r.POST("/alerts/:id/resolve", h.ResolveAlert)

	return r
}

func (h *Handler) Health(c *gin.Context) {
	c.JSON(200, gin.H{"status": "ok"})
}

func parseCompanyID(c *gin.Context) (int64, error) {
	val := strings.TrimSpace(c.Query("company_id"))
	if val == "" {
		return 1, nil
	}
	id, err := strconv.ParseInt(val, 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("%w: invalid company_id query parameter", domain.ErrValidation)
	}
	return id, nil
}

func handleUsecaseError(c *gin.Context, err error, notFoundMsg string) {
	if errors.Is(err, domain.ErrValidation) {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if errors.Is(err, domain.ErrForbidden) {
		c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		return
	}
	if errors.Is(err, domain.ErrConflict) {
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		return
	}
	if errors.Is(err, domain.ErrNotFound) {
		msg := err.Error()
		if notFoundMsg != "" {
			msg = notFoundMsg
		}
		c.JSON(http.StatusNotFound, gin.H{"error": msg})
		return
	}
	// 503/502: фиксированное тело без err.Error(), чтобы не раскрывать ответ внешнего провайдера (RFC 02, D5/D7).
	if errors.Is(err, domain.ErrUnavailable) {
		_ = c.Error(err)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "service unavailable"})
		return
	}
	if errors.Is(err, domain.ErrBadGateway) {
		_ = c.Error(err)
		c.JSON(http.StatusBadGateway, gin.H{"error": "bad gateway"})
		return
	}
	_ = c.Error(err)
	c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
}

// --- Accounts ---

func (h *Handler) List(c *gin.Context) {
	companyID, err := parseCompanyID(c)
	if err != nil {
		handleUsecaseError(c, err, "")
		return
	}

	if h.accountUsecase == nil {
		mockMu.RLock()
		accounts := make([]domain.Account, len(mockAccounts))
		copy(accounts, mockAccounts)
		mockMu.RUnlock()
		c.JSON(200, accounts)
		return
	}

	accounts, err := h.accountUsecase.List(c.Request.Context(), companyID)
	if err != nil {
		handleUsecaseError(c, err, "")
		return
	}
	if accounts == nil {
		accounts = make([]domain.Account, 0)
	}
	c.JSON(200, accounts)
}

func (h *Handler) GetByID(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		c.JSON(400, gin.H{"error": "id must be a positive number"})
		return
	}
	companyID, err := parseCompanyID(c)
	if err != nil {
		handleUsecaseError(c, err, "")
		return
	}

	if h.accountUsecase == nil {
		mockMu.RLock()
		var found *domain.Account
		for _, a := range mockAccounts {
			if a.ID == id {
				cp := a
				found = &cp
				break
			}
		}
		mockMu.RUnlock()
		if found != nil {
			c.JSON(200, *found)
			return
		}
		c.JSON(404, gin.H{"error": "account not found"})
		return
	}

	acc, err := h.accountUsecase.GetByID(c.Request.Context(), companyID, id)
	if err != nil {
		handleUsecaseError(c, err, "account not found")
		return
	}
	c.JSON(200, acc)
}

func (h *Handler) Create(c *gin.Context) {
	var req createAccountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	companyID := req.CompanyID
	if companyID <= 0 {
		var err error
		companyID, err = parseCompanyID(c)
		if err != nil {
			handleUsecaseError(c, err, "")
			return
		}
	}
	employeeID := req.EmployeeID
	if employeeID <= 0 {
		employeeID = 1
	}

	if h.accountUsecase == nil {
		mockMu.Lock()
		acc := domain.Account{
			ID:         int64(len(mockAccounts) + 1),
			CompanyID:  companyID,
			EmployeeID: employeeID,
			Phone:      req.Phone,
			Username:   req.Username,
			Status:     domain.StatusConnecting,
			CreatedAt:  time.Now().UTC(),
		}
		mockAccounts = append(mockAccounts, acc)
		mockMu.Unlock()
		c.JSON(201, acc)
		return
	}

	acc, err := h.accountUsecase.Create(c.Request.Context(), usecase.CreateAccountInput{
		CompanyID:  companyID,
		EmployeeID: employeeID,
		Phone:      req.Phone,
		Username:   req.Username,
	})
	if err != nil {
		handleUsecaseError(c, err, "")
		return
	}
	c.JSON(201, acc)
}

func (h *Handler) AssignEmployee(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		c.JSON(400, gin.H{"error": "id must be a positive number"})
		return
	}
	companyID, err := parseCompanyID(c)
	if err != nil {
		handleUsecaseError(c, err, "")
		return
	}

	var req assignAccountEmployeeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	if req.EmployeeID <= 0 {
		c.JSON(400, gin.H{"error": "employee_id must be a positive number"})
		return
	}

	if h.accountUsecase == nil {
		mockMu.Lock()
		found := false
		for i := range mockAccounts {
			if mockAccounts[i].ID == id {
				mockAccounts[i].EmployeeID = req.EmployeeID
				found = true
				break
			}
		}
		mockMu.Unlock()
		if !found {
			c.JSON(404, gin.H{"error": "account not found"})
			return
		}
		c.JSON(200, gin.H{"status": "assigned"})
		return
	}

	err = h.accountUsecase.AssignEmployee(c.Request.Context(), companyID, id, req.EmployeeID)
	if err != nil {
		handleUsecaseError(c, err, "account not found")
		return
	}
	c.JSON(200, gin.H{"status": "assigned"})
}

// --- Employees ---

func (h *Handler) ListEmployees(c *gin.Context) {
	companyID, err := parseCompanyID(c)
	if err != nil {
		handleUsecaseError(c, err, "")
		return
	}

	if h.employeeUsecase == nil {
		mockMu.RLock()
		employees := make([]domain.Employee, len(mockEmployees))
		copy(employees, mockEmployees)
		mockMu.RUnlock()
		c.JSON(200, employees)
		return
	}

	employees, err := h.employeeUsecase.List(c.Request.Context(), companyID)
	if err != nil {
		handleUsecaseError(c, err, "")
		return
	}
	if employees == nil {
		employees = make([]domain.Employee, 0)
	}
	c.JSON(200, employees)
}

func (h *Handler) GetEmployeeByID(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		c.JSON(400, gin.H{"error": "id must be a positive number"})
		return
	}
	companyID, err := parseCompanyID(c)
	if err != nil {
		handleUsecaseError(c, err, "")
		return
	}

	if h.employeeUsecase == nil {
		mockMu.RLock()
		var found *domain.Employee
		for _, e := range mockEmployees {
			if e.ID == id {
				cp := e
				found = &cp
				break
			}
		}
		mockMu.RUnlock()
		if found != nil {
			c.JSON(200, *found)
			return
		}
		c.JSON(404, gin.H{"error": "employee not found"})
		return
	}

	emp, err := h.employeeUsecase.GetByID(c.Request.Context(), companyID, id)
	if err != nil {
		handleUsecaseError(c, err, "employee not found")
		return
	}
	c.JSON(200, emp)
}

func (h *Handler) CreateEmployee(c *gin.Context) {
	var req createEmployeeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	companyID := req.CompanyID
	if companyID <= 0 {
		var err error
		companyID, err = parseCompanyID(c)
		if err != nil {
			handleUsecaseError(c, err, "")
			return
		}
	}

	if h.employeeUsecase == nil {
		mockMu.Lock()
		emp := domain.Employee{
			ID:        int64(len(mockEmployees) + 1),
			CompanyID: companyID,
			Name:      req.Name,
			Position:  req.Position,
			IsActive:  true,
		}
		mockEmployees = append(mockEmployees, emp)
		mockMu.Unlock()
		c.JSON(201, emp)
		return
	}

	emp, err := h.employeeUsecase.Create(c.Request.Context(), usecase.CreateEmployeeInput{
		CompanyID: companyID,
		Name:      req.Name,
		Position:  req.Position,
	})
	if err != nil {
		handleUsecaseError(c, err, "")
		return
	}
	c.JSON(201, emp)
}

func (h *Handler) UpdateEmployee(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		c.JSON(400, gin.H{"error": "id must be a positive number"})
		return
	}
	companyID, err := parseCompanyID(c)
	if err != nil {
		handleUsecaseError(c, err, "")
		return
	}

	var req updateEmployeeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	if h.employeeUsecase == nil {
		mockMu.Lock()
		found := false
		for i := range mockEmployees {
			if mockEmployees[i].ID == id {
				mockEmployees[i].Name = req.Name
				mockEmployees[i].Position = req.Position
				found = true
				break
			}
		}
		mockMu.Unlock()
		if !found {
			c.JSON(404, gin.H{"error": "employee not found"})
			return
		}
		c.JSON(200, gin.H{"status": "updated"})
		return
	}

	err = h.employeeUsecase.Update(c.Request.Context(), domain.Employee{
		ID:        id,
		CompanyID: companyID,
		Name:      req.Name,
		Position:  req.Position,
	})
	if err != nil {
		handleUsecaseError(c, err, "employee not found")
		return
	}
	c.JSON(200, gin.H{"status": "updated"})
}

func (h *Handler) DeactivateEmployee(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		c.JSON(400, gin.H{"error": "id must be a positive number"})
		return
	}
	companyID, err := parseCompanyID(c)
	if err != nil {
		handleUsecaseError(c, err, "")
		return
	}

	if h.employeeUsecase == nil {
		mockMu.Lock()
		found := false
		for i := range mockEmployees {
			if mockEmployees[i].ID == id {
				mockEmployees[i].IsActive = false
				found = true
				break
			}
		}
		mockMu.Unlock()
		if !found {
			c.JSON(404, gin.H{"error": "employee not found"})
			return
		}
		c.JSON(200, gin.H{"status": "deactivated"})
		return
	}

	err = h.employeeUsecase.Deactivate(c.Request.Context(), companyID, id)
	if err != nil {
		handleUsecaseError(c, err, "employee not found")
		return
	}
	c.JSON(200, gin.H{"status": "deactivated"})
}

// --- Chats ---

func parseChatFilter(c *gin.Context) (domain.ChatFilter, error) {
	companyID, err := parseCompanyID(c)
	if err != nil {
		return domain.ChatFilter{}, err
	}

	filter := domain.ChatFilter{
		CompanyID: companyID,
		Query:     strings.TrimSpace(c.Query("q")),
		SortBy:    domain.ChatSortLastMessage,
		Limit:     50,
		Offset:    0,
	}

	if s := c.Query("status"); s != "" {
		st := domain.ChatStatus(s)
		switch st {
		case domain.ChatStatusNewLead,
			domain.ChatStatusNegotiation,
			domain.ChatStatusWaitingReply,
			domain.ChatStatusInterested,
			domain.ChatStatusUnhappy,
			domain.ChatStatusLost,
			domain.ChatStatusActionRequired:
			filter.Status = &st
		default:
			return domain.ChatFilter{}, fmt.Errorf("%w: invalid status", domain.ErrValidation)
		}
	}

	if empStr := c.Query("employee_id"); empStr != "" {
		empID, err := strconv.ParseInt(empStr, 10, 64)
		if err != nil || empID <= 0 {
			return domain.ChatFilter{}, fmt.Errorf("%w: employee_id must be a positive number", domain.ErrValidation)
		}
		filter.EmployeeID = &empID
	}

	if accStr := c.Query("account_id"); accStr != "" {
		accID, err := strconv.ParseInt(accStr, 10, 64)
		if err != nil || accID <= 0 {
			return domain.ChatFilter{}, fmt.Errorf("%w: account_id must be a positive number", domain.ErrValidation)
		}
		filter.AccountID = &accID
	}

	if p := c.Query("priority"); p != "" {
		pr := domain.ChatPriority(p)
		switch pr {
		case domain.ChatPriorityLow, domain.ChatPriorityNormal, domain.ChatPriorityHigh:
			filter.Priority = &pr
		default:
			return domain.ChatFilter{}, fmt.Errorf("%w: invalid priority", domain.ErrValidation)
		}
	}

	if u := c.Query("unanswered"); u != "" {
		switch u {
		case "true":
			filter.UnansweredOnly = true
		case "false":
			filter.UnansweredOnly = false
		default:
			return domain.ChatFilter{}, fmt.Errorf("%w: unanswered must be true or false", domain.ErrValidation)
		}
	}

	if utf8.RuneCountInString(filter.Query) > 200 {
		return domain.ChatFilter{}, fmt.Errorf("%w: q must be at most 200 characters", domain.ErrValidation)
	}

	if sortBy := c.Query("sort"); sortBy != "" {
		switch sortBy {
		case domain.ChatSortLastMessage, domain.ChatSortAwaitingReply:
			filter.SortBy = sortBy
		default:
			return domain.ChatFilter{}, fmt.Errorf("%w: invalid sort", domain.ErrValidation)
		}
	}

	if limitStr := c.Query("limit"); limitStr != "" {
		limit, err := strconv.Atoi(limitStr)
		if err != nil || limit < 1 || limit > 100 {
			return domain.ChatFilter{}, fmt.Errorf("%w: limit must be between 1 and 100", domain.ErrValidation)
		}
		filter.Limit = limit
	}

	if offsetStr := c.Query("offset"); offsetStr != "" {
		offset, err := strconv.Atoi(offsetStr)
		if err != nil || offset < 0 {
			return domain.ChatFilter{}, fmt.Errorf("%w: offset must be a non-negative integer", domain.ErrValidation)
		}
		filter.Offset = offset
	}

	return filter, nil
}

func filterMockChats(filter domain.ChatFilter) []domain.Chat {
	lowerQuery := strings.ToLower(filter.Query)

	mockMu.RLock()
	res := make([]domain.Chat, 0)
	for _, chat := range mockChats {
		var chatAccount *domain.Account
		for i := range mockAccounts {
			if mockAccounts[i].ID == chat.AccountID && mockAccounts[i].CompanyID == filter.CompanyID {
				chatAccount = &mockAccounts[i]
				break
			}
		}
		if chatAccount == nil {
			continue
		}
		if filter.Status != nil && chat.Status != *filter.Status {
			continue
		}
		if filter.EmployeeID != nil && chatAccount.EmployeeID != *filter.EmployeeID {
			continue
		}
		if filter.AccountID != nil && chat.AccountID != *filter.AccountID {
			continue
		}
		if filter.Priority != nil && chat.Priority != *filter.Priority {
			continue
		}
		if filter.UnansweredOnly && chat.AwaitingReplySince == nil {
			continue
		}
		if lowerQuery != "" {
			matched := false
			for _, contact := range mockContacts {
				if contact.ID == chat.ContactID && contact.CompanyID == filter.CompanyID {
					if strings.Contains(strings.ToLower(contact.Name), lowerQuery) ||
						strings.Contains(strings.ToLower(contact.Username), lowerQuery) {
						matched = true
					}
					break
				}
			}
			if !matched {
				for _, msg := range mockMessages {
					if msg.ChatID == chat.ID && strings.Contains(strings.ToLower(msg.Text), lowerQuery) {
						matched = true
						break
					}
				}
			}
			if !matched {
				continue
			}
		}
		res = append(res, chat)
	}
	mockMu.RUnlock()

	sort.Slice(res, func(i, j int) bool {
		if filter.SortBy == domain.ChatSortAwaitingReply {
			ai := res[i].AwaitingReplySince
			aj := res[j].AwaitingReplySince
			if ai == nil && aj == nil {
				return res[i].ID > res[j].ID
			}
			if ai == nil {
				return false
			}
			if aj == nil {
				return true
			}
			if ai.Equal(*aj) {
				return res[i].ID > res[j].ID
			}
			return ai.Before(*aj)
		}
		if res[i].LastMessageAt.Equal(res[j].LastMessageAt) {
			return res[i].ID > res[j].ID
		}
		return res[i].LastMessageAt.After(res[j].LastMessageAt)
	})

	if filter.Offset >= len(res) {
		return make([]domain.Chat, 0)
	}
	end := filter.Offset + filter.Limit
	if end > len(res) {
		end = len(res)
	}
	return res[filter.Offset:end]
}

func (h *Handler) ListChats(c *gin.Context) {
	filter, err := parseChatFilter(c)
	if err != nil {
		handleUsecaseError(c, err, "")
		return
	}

	if h.chatUsecase == nil {
		c.JSON(http.StatusOK, filterMockChats(filter))
		return
	}

	chats, err := h.chatUsecase.ListByFilter(c.Request.Context(), filter)
	if err != nil {
		handleUsecaseError(c, err, "chats not found")
		return
	}
	if chats == nil {
		chats = make([]domain.Chat, 0)
	}
	c.JSON(http.StatusOK, chats)
}

func (h *Handler) GetChatByID(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		c.JSON(400, gin.H{"error": "id must be a positive number"})
		return
	}
	companyID, err := parseCompanyID(c)
	if err != nil {
		handleUsecaseError(c, err, "")
		return
	}

	if h.chatUsecase == nil {
		mockMu.RLock()
		var found *usecase.ChatDetails
		for _, chat := range mockChats {
			if chat.ID == id {
				details := &usecase.ChatDetails{
					Chat: chat,
				}
				for _, contact := range mockContacts {
					if contact.ID == chat.ContactID {
						contactCopy := contact
						details.Contact = &contactCopy
						break
					}
				}
				found = details
				break
			}
		}
		mockMu.RUnlock()
		if found != nil {
			c.JSON(200, found)
			return
		}
		c.JSON(404, gin.H{"error": "chat not found"})
		return
	}

	details, err := h.chatUsecase.GetDetails(c.Request.Context(), companyID, id)
	if err != nil {
		handleUsecaseError(c, err, "chat not found")
		return
	}
	c.JSON(200, details)
}

func (h *Handler) UpdateChatStatus(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		c.JSON(400, gin.H{"error": "id must be a positive number"})
		return
	}
	companyID, err := parseCompanyID(c)
	if err != nil {
		handleUsecaseError(c, err, "")
		return
	}

	var req updateChatStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	if h.chatUsecase == nil {
		switch req.Status {
		case domain.ChatStatusNewLead,
			domain.ChatStatusNegotiation,
			domain.ChatStatusWaitingReply,
			domain.ChatStatusInterested,
			domain.ChatStatusUnhappy,
			domain.ChatStatusLost,
			domain.ChatStatusActionRequired:
		default:
			c.JSON(400, gin.H{"error": "invalid status"})
			return
		}

		mockMu.Lock()
		found := false
		for i := range mockChats {
			if mockChats[i].ID == id {
				mockChats[i].Status = req.Status
				mockChats[i].StatusSource = domain.StatusSourceManual
				found = true
				break
			}
		}
		mockMu.Unlock()
		if !found {
			c.JSON(404, gin.H{"error": "chat not found"})
			return
		}
		c.JSON(200, gin.H{"status": "updated"})
		return
	}

	err = h.chatUsecase.SetManualStatus(c.Request.Context(), companyID, id, req.Status)
	if err != nil {
		handleUsecaseError(c, err, "chat not found")
		return
	}
	c.JSON(200, gin.H{"status": "updated"})
}

// --- Contacts ---

func (h *Handler) ListContacts(c *gin.Context) {
	companyID, err := parseCompanyID(c)
	if err != nil {
		handleUsecaseError(c, err, "")
		return
	}

	if h.contactUsecase == nil {
		mockMu.RLock()
		contacts := make([]domain.Contact, len(mockContacts))
		copy(contacts, mockContacts)
		mockMu.RUnlock()
		c.JSON(200, contacts)
		return
	}

	contacts, err := h.contactUsecase.ListByCompany(c.Request.Context(), companyID)
	if err != nil {
		handleUsecaseError(c, err, "")
		return
	}
	if contacts == nil {
		contacts = make([]domain.Contact, 0)
	}
	c.JSON(200, contacts)
}

func (h *Handler) GetContactByID(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		c.JSON(400, gin.H{"error": "id must be a positive number"})
		return
	}
	companyID, err := parseCompanyID(c)
	if err != nil {
		handleUsecaseError(c, err, "")
		return
	}

	if h.contactUsecase == nil {
		mockMu.RLock()
		var found *domain.Contact
		for _, contact := range mockContacts {
			if contact.ID == id {
				cp := contact
				found = &cp
				break
			}
		}
		mockMu.RUnlock()
		if found != nil {
			c.JSON(200, *found)
			return
		}
		c.JSON(404, gin.H{"error": "contact not found"})
		return
	}

	contact, err := h.contactUsecase.GetByID(c.Request.Context(), companyID, id)
	if err != nil {
		handleUsecaseError(c, err, "contact not found")
		return
	}
	c.JSON(200, contact)
}

// --- Messages ---

func (h *Handler) ListMessagesByChat(c *gin.Context) {
	chatID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || chatID <= 0 {
		c.JSON(400, gin.H{"error": "id must be a positive number"})
		return
	}
	companyID, err := parseCompanyID(c)
	if err != nil {
		handleUsecaseError(c, err, "")
		return
	}

	limit := 50
	if l := c.Query("limit"); l != "" {
		parsedLimit, err := strconv.Atoi(l)
		if err != nil || parsedLimit <= 0 || parsedLimit > 100 {
			c.JSON(400, gin.H{"error": "limit must be between 1 and 100"})
			return
		}
		limit = parsedLimit
	}

	offset := 0
	if o := c.Query("offset"); o != "" {
		parsedOffset, err := strconv.Atoi(o)
		if err != nil || parsedOffset < 0 {
			c.JSON(400, gin.H{"error": "offset must be a non-negative number"})
			return
		}
		offset = parsedOffset
	}

	if h.messageUsecase == nil {
		mockMu.RLock()
		res := make([]domain.Message, 0)
		for _, m := range mockMessages {
			if m.ChatID == chatID {
				res = append(res, m)
			}
		}
		mockMu.RUnlock()
		c.JSON(200, res)
		return
	}

	messages, err := h.messageUsecase.ListByChat(c.Request.Context(), companyID, chatID, limit, offset)
	if err != nil {
		handleUsecaseError(c, err, "")
		return
	}
	if messages == nil {
		messages = make([]domain.Message, 0)
	}
	c.JSON(200, messages)
}

// --- Analytics ---

func parseDateRange(fromStr, toStr string) (time.Time, time.Time, error) {
	var from, to time.Time
	var err error

	if toStr != "" {
		to, err = time.Parse("2006-01-02", toStr)
		if err != nil {
			return time.Time{}, time.Time{}, errors.New("to must be in format YYYY-MM-DD")
		}
	} else {
		now := time.Now().UTC()
		to = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	}

	if fromStr != "" {
		from, err = time.Parse("2006-01-02", fromStr)
		if err != nil {
			return time.Time{}, time.Time{}, errors.New("from must be in format YYYY-MM-DD")
		}
	} else {
		from = to.AddDate(0, 0, -30)
	}

	if from.After(to) {
		return time.Time{}, time.Time{}, errors.New("from date must not be after to date")
	}

	return from, to, nil
}

func (h *Handler) GetDashboard(c *gin.Context) {
	from, to, err := parseDateRange(c.Query("from"), c.Query("to"))
	if err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	companyID, err := parseCompanyID(c)
	if err != nil {
		handleUsecaseError(c, err, "")
		return
	}

	if h.accountDailyStatsUsecase == nil {
		mockMu.RLock()
		stats := mockDashboard
		mockMu.RUnlock()
		c.JSON(200, stats)
		return
	}

	stats, err := h.accountDailyStatsUsecase.GetDashboard(c.Request.Context(), companyID, from, to)
	if err != nil {
		handleUsecaseError(c, err, "")
		return
	}
	c.JSON(200, stats)
}

func (h *Handler) GetAccountRatings(c *gin.Context) {
	from, to, err := parseDateRange(c.Query("from"), c.Query("to"))
	if err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	companyID, err := parseCompanyID(c)
	if err != nil {
		handleUsecaseError(c, err, "")
		return
	}

	if h.accountDailyStatsUsecase == nil {
		mockMu.RLock()
		ratings := make([]domain.AccountRating, len(mockAccountRatings))
		copy(ratings, mockAccountRatings)
		mockMu.RUnlock()
		c.JSON(200, ratings)
		return
	}

	ratings, err := h.accountDailyStatsUsecase.GetAccountRatings(c.Request.Context(), companyID, from, to)
	if err != nil {
		handleUsecaseError(c, err, "")
		return
	}
	if ratings == nil {
		ratings = make([]domain.AccountRating, 0)
	}
	c.JSON(200, ratings)
}

// --- Companies ---

func (h *Handler) CreateCompany(c *gin.Context) {
	var req createCompanyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	if req.NoReplyThreshold < 0 {
		c.JSON(400, gin.H{"error": "no_reply_threshold cannot be negative"})
		return
	}

	if h.companyUsecase == nil {
		c.JSON(500, gin.H{"error": "company service unavailable"})
		return
	}

	id, err := h.companyUsecase.Create(c.Request.Context(), domain.Company{
		Name:             req.Name,
		Timezone:         req.Timezone,
		NoReplyThreshold: time.Duration(req.NoReplyThreshold),
	})
	if err != nil {
		handleUsecaseError(c, err, "")
		return
	}
	c.JSON(201, gin.H{"id": id, "name": req.Name})
}

func (h *Handler) GetCompanyByID(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		c.JSON(400, gin.H{"error": "id must be a positive number"})
		return
	}

	if h.companyUsecase == nil {
		c.JSON(500, gin.H{"error": "company service unavailable"})
		return
	}

	comp, err := h.companyUsecase.GetByID(c.Request.Context(), id)
	if err != nil {
		handleUsecaseError(c, err, "company not found")
		return
	}
	c.JSON(200, comp)
}

// --- Chat Summaries ---

func (h *Handler) GetChatSummary(c *gin.Context) {
	chatID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || chatID <= 0 {
		c.JSON(400, gin.H{"error": "id must be a positive number"})
		return
	}
	companyID, err := parseCompanyID(c)
	if err != nil {
		handleUsecaseError(c, err, "")
		return
	}

	if h.summaryUsecase == nil {
		c.JSON(500, gin.H{"error": "summary service unavailable"})
		return
	}

	summary, err := h.summaryUsecase.GetByChatID(c.Request.Context(), companyID, chatID)
	if err != nil {
		handleUsecaseError(c, err, "summary not found")
		return
	}
	c.JSON(200, summary)
}

func (h *Handler) UpsertChatSummary(c *gin.Context) {
	chatID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || chatID <= 0 {
		c.JSON(400, gin.H{"error": "id must be a positive number"})
		return
	}
	companyID, err := parseCompanyID(c)
	if err != nil {
		handleUsecaseError(c, err, "")
		return
	}

	var req upsertChatSummaryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	if h.summaryUsecase == nil {
		c.JSON(500, gin.H{"error": "summary service unavailable"})
		return
	}

	summary := domain.ChatSummary{
		ChatID:        chatID,
		Topic:         req.Topic,
		Agreements:    req.Agreements,
		NextStep:      req.NextStep,
		OpenQuestions: req.OpenQuestions,
		Risk:          req.Risk,
		LastMessageID: req.LastMessageID,
		GeneratedAt:   time.Now().UTC(),
	}

	if err := h.summaryUsecase.Upsert(c.Request.Context(), companyID, summary); err != nil {
		handleUsecaseError(c, err, "chat not found")
		return
	}
	c.JSON(200, gin.H{"status": "saved"})
}

// GenerateChatSummary строит AI-саммари чата (RFC 02, п.4.1). Request body игнорируется.
// force: отсутствует → false; ровно одно значение "true"/"false"; иное (в т.ч. пустое) → 400.
func (h *Handler) GenerateChatSummary(c *gin.Context) {
	chatID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || chatID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id must be a positive number"})
		return
	}

	query, err := url.ParseQuery(c.Request.URL.RawQuery)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid query parameters"})
		return
	}

	force := false
	if vals, ok := query["force"]; ok {
		if len(vals) != 1 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "force must be true or false"})
			return
		}
		switch vals[0] {
		case "true":
			force = true
		case "false":
			force = false
		default:
			c.JSON(http.StatusBadRequest, gin.H{"error": "force must be true or false"})
			return
		}
	}

	companyID, err := parseCompanyID(c)
	if err != nil {
		handleUsecaseError(c, err, "")
		return
	}

	if h.summaryUsecase == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "summary service unavailable"})
		return
	}

	summary, err := h.summaryUsecase.Generate(c.Request.Context(), companyID, chatID, force)
	if err != nil {
		handleUsecaseError(c, err, "chat not found")
		return
	}
	c.JSON(http.StatusOK, summary)
}

// --- Users ---

func (h *Handler) ListUsers(c *gin.Context) {
	companyID, err := parseCompanyID(c)
	if err != nil {
		handleUsecaseError(c, err, "")
		return
	}

	if h.userUsecase == nil {
		c.JSON(500, gin.H{"error": "user service unavailable"})
		return
	}

	users, err := h.userUsecase.ListByCompany(c.Request.Context(), companyID)
	if err != nil {
		handleUsecaseError(c, err, "")
		return
	}
	if users == nil {
		users = make([]domain.User, 0)
	}
	c.JSON(200, users)
}

func (h *Handler) GetUserByID(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		c.JSON(400, gin.H{"error": "id must be a positive number"})
		return
	}
	companyID, err := parseCompanyID(c)
	if err != nil {
		handleUsecaseError(c, err, "")
		return
	}

	if h.userUsecase == nil {
		c.JSON(500, gin.H{"error": "user service unavailable"})
		return
	}

	user, err := h.userUsecase.GetByID(c.Request.Context(), companyID, id)
	if err != nil {
		handleUsecaseError(c, err, "user not found")
		return
	}
	c.JSON(200, user)
}

func (h *Handler) CreateUser(c *gin.Context) {
	var req createUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	companyID := req.CompanyID
	if companyID <= 0 {
		var err error
		companyID, err = parseCompanyID(c)
		if err != nil {
			handleUsecaseError(c, err, "")
			return
		}
	}

	if h.userUsecase == nil {
		c.JSON(500, gin.H{"error": "user service unavailable"})
		return
	}

	user := domain.User{
		CompanyID:    companyID,
		Email:        req.Email,
		PasswordHash: req.PasswordHash,
		Role:         req.Role,
		EmployeeID:   req.EmployeeID,
		CreatedAt:    time.Now().UTC(),
	}

	id, err := h.userUsecase.Create(c.Request.Context(), user)
	if err != nil {
		handleUsecaseError(c, err, "")
		return
	}
	c.JSON(201, gin.H{"id": id, "email": user.Email})
}

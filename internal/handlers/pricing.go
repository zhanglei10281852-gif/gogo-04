package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"venue-booking-admin/internal/auth"
	"venue-booking-admin/internal/models"
	"venue-booking-admin/internal/pricing"
)

// PricingHandler 定价处理器
type PricingHandler struct {
	DB        *gorm.DB
	Engine    *pricing.Engine
	LockSvc   *pricing.LockService
	AuditSvc  *pricing.AuditService
	AnalysisSvc *pricing.AnalysisService
}

// NewPricingHandler 创建定价处理器
func NewPricingHandler(db *gorm.DB) *PricingHandler {
	engine := pricing.NewEngine(db)
	return &PricingHandler{
		DB:         db,
		Engine:     engine,
		LockSvc:    pricing.NewLockService(db, engine),
		AuditSvc:   pricing.NewAuditService(db),
		AnalysisSvc: pricing.NewAnalysisService(db),
	}
}

// ---------- 定价规则管理 ----------

type pricingRuleReq struct {
	Name        string              `json:"name" binding:"required"`
	RuleType    models.RuleType     `json:"rule_type" binding:"required"`
	Description string              `json:"description"`
	Operator    models.RuleOperator `json:"operator" binding:"required"`
	Value       float64             `json:"value" binding:"required"`
	IsActive    *bool               `json:"is_active"`
	Config      interface{}         `json:"config"`
}

func (h *PricingHandler) ListPricingRules(c *gin.Context) {
	var rules []models.PricingRule
	q := h.DB.Order("id desc")

	if ruleType := c.Query("rule_type"); ruleType != "" {
		q = q.Where("rule_type = ?", ruleType)
	}
	if active := c.Query("is_active"); active != "" {
		q = q.Where("is_active = ?", active == "true")
	}

	q.Find(&rules)
	c.JSON(http.StatusOK, rules)
}

func (h *PricingHandler) CreatePricingRule(c *gin.Context) {
	var req pricingRuleReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"detail": "请求参数不合法"})
		return
	}

	configJSON, _ := json.Marshal(req.Config)

	isActive := true
	if req.IsActive != nil {
		isActive = *req.IsActive
	}

	rule := models.PricingRule{
		Name:        req.Name,
		RuleType:    req.RuleType,
		Description: req.Description,
		Operator:    req.Operator,
		Value:       req.Value,
		IsActive:    isActive,
		Config:      string(configJSON),
	}

	if err := h.DB.Create(&rule).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"detail": "创建规则失败"})
		return
	}

	user := c.MustGet("user").(models.User)
	h.AuditSvc.LogAction(0, "create_rule", user.ID, user.Username, nil, rule, c.ClientIP())

	c.JSON(http.StatusCreated, rule)
}

func (h *PricingHandler) GetPricingRule(c *gin.Context) {
	var rule models.PricingRule
	if err := h.DB.First(&rule, c.Param("id")).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"detail": "规则不存在"})
		return
	}
	c.JSON(http.StatusOK, rule)
}

func (h *PricingHandler) UpdatePricingRule(c *gin.Context) {
	var rule models.PricingRule
	if err := h.DB.First(&rule, c.Param("id")).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"detail": "规则不存在"})
		return
	}

	beforeRule := rule

	var req pricingRuleReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"detail": "请求参数不合法"})
		return
	}

	configJSON, _ := json.Marshal(req.Config)

	rule.Name = req.Name
	rule.RuleType = req.RuleType
	rule.Description = req.Description
	rule.Operator = req.Operator
	rule.Value = req.Value
	rule.Config = string(configJSON)
	if req.IsActive != nil {
		rule.IsActive = *req.IsActive
	}

	if err := h.DB.Save(&rule).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"detail": "更新规则失败"})
		return
	}

	user := c.MustGet("user").(models.User)
	h.AuditSvc.LogAction(0, "update_rule", user.ID, user.Username, beforeRule, rule, c.ClientIP())

	c.JSON(http.StatusOK, rule)
}

func (h *PricingHandler) DeletePricingRule(c *gin.Context) {
	var rule models.PricingRule
	if err := h.DB.First(&rule, c.Param("id")).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"detail": "规则不存在"})
		return
	}

	h.DB.Delete(&rule)

	user := c.MustGet("user").(models.User)
	h.AuditSvc.LogAction(0, "delete_rule", user.ID, user.Username, rule, nil, c.ClientIP())

	c.Status(http.StatusNoContent)
}

// ---------- 规则绑定管理 ----------

type bindingReq struct {
	VenueID  uint    `json:"venue_id" binding:"required"`
	RuleID   *uint   `json:"rule_id"` // 可空：仅设置保底封顶时不传
	Priority int     `json:"priority"`
	IsActive *bool   `json:"is_active"`
	MinPrice float64 `json:"min_price"`
	MaxPrice float64 `json:"max_price"`
}

func (h *PricingHandler) ListVenueBindings(c *gin.Context) {
	venueID := c.Query("venue_id")
	var bindings []models.VenuePricingBinding

	q := h.DB.Preload("Rule").Order("priority DESC, id ASC")
	if venueID != "" {
		q = q.Where("venue_id = ?", venueID)
	}

	q.Find(&bindings)
	c.JSON(http.StatusOK, bindings)
}

func (h *PricingHandler) CreateBinding(c *gin.Context) {
	var req bindingReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"detail": "请求参数不合法"})
		return
	}

	isActive := true
	if req.IsActive != nil {
		isActive = *req.IsActive
	}

	var ruleID *uint
	if req.RuleID != nil && *req.RuleID > 0 {
		ruleID = req.RuleID
	}

	binding := models.VenuePricingBinding{
		VenueID:  req.VenueID,
		RuleID:   ruleID,
		Priority: req.Priority,
		IsActive: isActive,
		MinPrice: req.MinPrice,
		MaxPrice: req.MaxPrice,
	}

	if err := h.DB.Create(&binding).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"detail": "创建绑定失败"})
		return
	}

	h.DB.Preload("Rule").First(&binding, binding.ID)

	user := c.MustGet("user").(models.User)
	h.AuditSvc.LogAction(req.VenueID, "bind_rule", user.ID, user.Username, nil, binding, c.ClientIP())

	c.JSON(http.StatusCreated, binding)
}

func (h *PricingHandler) UpdateBinding(c *gin.Context) {
	var binding models.VenuePricingBinding
	if err := h.DB.First(&binding, c.Param("id")).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"detail": "绑定不存在"})
		return
	}

	beforeBinding := binding

	var req bindingReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"detail": "请求参数不合法"})
		return
	}

	binding.Priority = req.Priority
	binding.MinPrice = req.MinPrice
	binding.MaxPrice = req.MaxPrice
	if req.IsActive != nil {
		binding.IsActive = *req.IsActive
	}
	if req.RuleID != nil {
		if *req.RuleID > 0 {
			binding.RuleID = req.RuleID
		} else {
			binding.RuleID = nil
		}
	}

	if err := h.DB.Save(&binding).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"detail": "更新绑定失败"})
		return
	}

	h.DB.Preload("Rule").First(&binding, binding.ID)

	user := c.MustGet("user").(models.User)
	h.AuditSvc.LogAction(binding.VenueID, "update_binding", user.ID, user.Username, beforeBinding, binding, c.ClientIP())

	c.JSON(http.StatusOK, binding)
}

func (h *PricingHandler) DeleteBinding(c *gin.Context) {
	var binding models.VenuePricingBinding
	if err := h.DB.First(&binding, c.Param("id")).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"detail": "绑定不存在"})
		return
	}

	venueID := binding.VenueID
	h.DB.Delete(&binding)

	user := c.MustGet("user").(models.User)
	h.AuditSvc.LogAction(venueID, "unbind_rule", user.ID, user.Username, binding, nil, c.ClientIP())

	c.Status(http.StatusNoContent)
}

// ---------- 实时查价 ----------

type priceQueryReq struct {
	VenueID     uint    `json:"venue_id" binding:"required"`
	BookDate    string  `json:"book_date" binding:"required"`
	StartHour   int     `json:"start_hour" binding:"required"`
	EndHour     int     `json:"end_hour" binding:"required"`
	MemberLevel string  `json:"member_level"`
	Occupancy   float64 `json:"occupancy"`
}

func (h *PricingHandler) CalculatePrice(c *gin.Context) {
	var req priceQueryReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"detail": "请求参数不合法"})
		return
	}

	query := pricing.PriceQuery{
		VenueID:     req.VenueID,
		BookDate:    req.BookDate,
		StartHour:   req.StartHour,
		EndHour:     req.EndHour,
		MemberLevel: req.MemberLevel,
	}

	if req.Occupancy > 0 {
		query.Occupancy = &req.Occupancy
	}

	result, err := h.Engine.CalculatePrice(query)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"detail": err.Error()})
		return
	}

	c.JSON(http.StatusOK, result)
}

// ---------- 价格锁 ----------

type lockPriceReq struct {
	VenueID     uint   `json:"venue_id" binding:"required"`
	BookDate    string `json:"book_date" binding:"required"`
	StartHour   int    `json:"start_hour" binding:"required"`
	EndHour     int    `json:"end_hour" binding:"required"`
	MemberLevel string `json:"member_level"`
}

func (h *PricingHandler) LockPrice(c *gin.Context) {
	var req lockPriceReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"detail": "请求参数不合法"})
		return
	}

	query := pricing.PriceQuery{
		VenueID:     req.VenueID,
		BookDate:    req.BookDate,
		StartHour:   req.StartHour,
		EndHour:     req.EndHour,
		MemberLevel: req.MemberLevel,
	}

	lock, err := h.LockSvc.LockPrice(query, c.ClientIP())
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"detail": err.Error()})
		return
	}

	c.JSON(http.StatusOK, lock)
}

func (h *PricingHandler) GetLockedPrice(c *gin.Context) {
	lockKey := c.Param("lock_key")
	lock, err := h.LockSvc.GetLockedPrice(lockKey)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"detail": err.Error()})
		return
	}
	c.JSON(http.StatusOK, lock)
}

// ---------- 节假日管理 ----------

type holidayReq struct {
	Date      string `json:"date" binding:"required"`
	Name      string `json:"name" binding:"required"`
	IsWorkday bool   `json:"is_workday"`
}

func (h *PricingHandler) ListHolidays(c *gin.Context) {
	var holidays []models.Holiday
	q := h.DB.Order("date ASC")

	if year := c.Query("year"); year != "" {
		q = q.Where("date LIKE ?", year+"%")
	}

	q.Find(&holidays)
	c.JSON(http.StatusOK, holidays)
}

func (h *PricingHandler) CreateHoliday(c *gin.Context) {
	var req holidayReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"detail": "请求参数不合法"})
		return
	}

	holiday := models.Holiday{
		Date:      req.Date,
		Name:      req.Name,
		IsWorkday: req.IsWorkday,
	}

	if err := h.DB.Create(&holiday).Error; err != nil {
		c.JSON(http.StatusConflict, gin.H{"detail": "该日期已存在"})
		return
	}

	c.JSON(http.StatusCreated, holiday)
}

func (h *PricingHandler) UpdateHoliday(c *gin.Context) {
	var holiday models.Holiday
	if err := h.DB.First(&holiday, c.Param("id")).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"detail": "节假日不存在"})
		return
	}

	var req holidayReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"detail": "请求参数不合法"})
		return
	}

	holiday.Date = req.Date
	holiday.Name = req.Name
	holiday.IsWorkday = req.IsWorkday

	if err := h.DB.Save(&holiday).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"detail": "更新失败"})
		return
	}

	c.JSON(http.StatusOK, holiday)
}

func (h *PricingHandler) DeleteHoliday(c *gin.Context) {
	var holiday models.Holiday
	if err := h.DB.First(&holiday, c.Param("id")).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"detail": "节假日不存在"})
		return
	}

	h.DB.Delete(&holiday)
	c.Status(http.StatusNoContent)
}

// ---------- 价格审计 ----------

func (h *PricingHandler) ListAuditLogs(c *gin.Context) {
	venueID, _ := strconv.ParseUint(c.Query("venue_id"), 10, 32)
	action := c.Query("action")

	startTime := time.Time{}
	if s := c.Query("start_time"); s != "" {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			startTime = t
		}
	}

	endTime := time.Time{}
	if e := c.Query("end_time"); e != "" {
		if t, err := time.Parse(time.RFC3339, e); err == nil {
			endTime = t
		}
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	logs, total, err := h.AuditSvc.ListAuditLogs(uint(venueID), action, startTime, endTime, page, pageSize)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"detail": "查询失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"items": logs,
		"total": total,
		"page":  page,
		"page_size": pageSize,
	})
}

// ---------- 调价模拟 ----------

type simulateReq struct {
	VenueID     uint    `json:"venue_id" binding:"required"`
	Date        string  `json:"date" binding:"required"`
	Occupancy   float64 `json:"occupancy"`
	MemberLevel string  `json:"member_level"`
}

func (h *PricingHandler) SimulateDayPrice(c *gin.Context) {
	var req simulateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"detail": "请求参数不合法"})
		return
	}

	result, err := h.AnalysisSvc.SimulateRevenueImpact(req.VenueID, req.Date, req.Occupancy, req.MemberLevel)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"detail": err.Error()})
		return
	}

	c.JSON(http.StatusOK, result)
}

// ---------- 促销活动管理 ----------

type promotionReq struct {
	Name          string    `json:"name" binding:"required"`
	PromotionType string    `json:"promotion_type" binding:"required"`
	Description   string    `json:"description"`
	StartTime     string    `json:"start_time" binding:"required"`
	EndTime       string    `json:"end_time" binding:"required"`
	VenueIDs      []uint    `json:"venue_ids"`
	Weekdays      []int     `json:"weekdays"`
	DiscountRate  float64   `json:"discount_rate"`
	FullAmount    float64   `json:"full_amount"`
	ReduceAmount  float64   `json:"reduce_amount"`
	IsActive      *bool     `json:"is_active"`
}

func (h *PricingHandler) ListPromotions(c *gin.Context) {
	var promotions []models.Promotion
	q := h.DB.Order("id desc")

	if ptype := c.Query("promotion_type"); ptype != "" {
		q = q.Where("promotion_type = ?", ptype)
	}
	if active := c.Query("is_active"); active != "" {
		q = q.Where("is_active = ?", active == "true")
	}

	q.Find(&promotions)
	c.JSON(http.StatusOK, promotions)
}

func (h *PricingHandler) CreatePromotion(c *gin.Context) {
	var req promotionReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"detail": "请求参数不合法"})
		return
	}

	startTime, _ := time.Parse(time.RFC3339, req.StartTime)
	endTime, _ := time.Parse(time.RFC3339, req.EndTime)

	venueIDsJSON, _ := json.Marshal(req.VenueIDs)
	weekdaysJSON, _ := json.Marshal(req.Weekdays)

	isActive := true
	if req.IsActive != nil {
		isActive = *req.IsActive
	}

	promo := models.Promotion{
		Name:          req.Name,
		PromotionType: models.PromotionType(req.PromotionType),
		Description:   req.Description,
		StartTime:     startTime,
		EndTime:       endTime,
		VenueIDs:      string(venueIDsJSON),
		Weekdays:      string(weekdaysJSON),
		DiscountRate:  req.DiscountRate,
		FullAmount:    req.FullAmount,
		ReduceAmount:  req.ReduceAmount,
		IsActive:      isActive,
	}

	if err := h.DB.Create(&promo).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"detail": "创建促销活动失败"})
		return
	}

	c.JSON(http.StatusCreated, promo)
}

func (h *PricingHandler) GetPromotion(c *gin.Context) {
	var promo models.Promotion
	if err := h.DB.First(&promo, c.Param("id")).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"detail": "促销活动不存在"})
		return
	}
	c.JSON(http.StatusOK, promo)
}

func (h *PricingHandler) UpdatePromotion(c *gin.Context) {
	var promo models.Promotion
	if err := h.DB.First(&promo, c.Param("id")).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"detail": "促销活动不存在"})
		return
	}

	var req promotionReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"detail": "请求参数不合法"})
		return
	}

	startTime, _ := time.Parse(time.RFC3339, req.StartTime)
	endTime, _ := time.Parse(time.RFC3339, req.EndTime)

	venueIDsJSON, _ := json.Marshal(req.VenueIDs)
	weekdaysJSON, _ := json.Marshal(req.Weekdays)

	promo.Name = req.Name
	promo.PromotionType = models.PromotionType(req.PromotionType)
	promo.Description = req.Description
	promo.StartTime = startTime
	promo.EndTime = endTime
	promo.VenueIDs = string(venueIDsJSON)
	promo.Weekdays = string(weekdaysJSON)
	promo.DiscountRate = req.DiscountRate
	promo.FullAmount = req.FullAmount
	promo.ReduceAmount = req.ReduceAmount
	if req.IsActive != nil {
		promo.IsActive = *req.IsActive
	}

	if err := h.DB.Save(&promo).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"detail": "更新促销活动失败"})
		return
	}

	c.JSON(http.StatusOK, promo)
}

func (h *PricingHandler) DeletePromotion(c *gin.Context) {
	var promo models.Promotion
	if err := h.DB.First(&promo, c.Param("id")).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"detail": "促销活动不存在"})
		return
	}

	h.DB.Delete(&promo)
	c.Status(http.StatusNoContent)
}

// ---------- 灰度发布管理 ----------

type grayReleaseReq struct {
	Name        string `json:"name" binding:"required"`
	Description string `json:"description"`
	Strategy    string `json:"strategy" binding:"required"`
	VenueList   []uint `json:"venue_list"`
	Percentage  int    `json:"percentage"`
	IsActive    *bool  `json:"is_active"`
}

func (h *PricingHandler) ListGrayReleases(c *gin.Context) {
	var releases []models.GrayRelease
	h.DB.Order("id desc").Find(&releases)
	c.JSON(http.StatusOK, releases)
}

func (h *PricingHandler) CreateGrayRelease(c *gin.Context) {
	var req grayReleaseReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"detail": "请求参数不合法"})
		return
	}

	venueListJSON, _ := json.Marshal(req.VenueList)

	isActive := false
	if req.IsActive != nil {
		isActive = *req.IsActive
	}

	release := models.GrayRelease{
		Name:        req.Name,
		Description: req.Description,
		Strategy:    req.Strategy,
		VenueList:   string(venueListJSON),
		Percentage:  req.Percentage,
		IsActive:    isActive,
	}

	if err := h.DB.Create(&release).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"detail": "创建灰度发布失败"})
		return
	}

	c.JSON(http.StatusCreated, release)
}

func (h *PricingHandler) UpdateGrayRelease(c *gin.Context) {
	var release models.GrayRelease
	if err := h.DB.First(&release, c.Param("id")).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"detail": "灰度发布不存在"})
		return
	}

	var req grayReleaseReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"detail": "请求参数不合法"})
		return
	}

	venueListJSON, _ := json.Marshal(req.VenueList)

	release.Name = req.Name
	release.Description = req.Description
	release.Strategy = req.Strategy
	release.VenueList = string(venueListJSON)
	release.Percentage = req.Percentage
	if req.IsActive != nil {
		release.IsActive = *req.IsActive
	}

	if err := h.DB.Save(&release).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"detail": "更新灰度发布失败"})
		return
	}

	c.JSON(http.StatusOK, release)
}

func (h *PricingHandler) DeleteGrayRelease(c *gin.Context) {
	var release models.GrayRelease
	if err := h.DB.First(&release, c.Param("id")).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"detail": "灰度发布不存在"})
		return
	}

	h.DB.Delete(&release)
	c.Status(http.StatusNoContent)
}

// ---------- 定价效果分析 ----------

func (h *PricingHandler) GetPriceDistribution(c *gin.Context) {
	venueID, _ := strconv.ParseUint(c.Query("venue_id"), 10, 32)
	startDate := c.Query("start_date")
	endDate := c.Query("end_date")

	result, err := h.AnalysisSvc.GetPriceDistribution(uint(venueID), startDate, endDate, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"detail": "查询失败"})
		return
	}

	c.JSON(http.StatusOK, result)
}

func (h *PricingHandler) GetRevenueAnalysis(c *gin.Context) {
	venueID, _ := strconv.ParseUint(c.Query("venue_id"), 10, 32)
	startDate := c.Query("start_date")
	endDate := c.Query("end_date")

	result, err := h.AnalysisSvc.GetRevenueAnalysis(uint(venueID), startDate, endDate)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"detail": "查询失败"})
		return
	}

	c.JSON(http.StatusOK, result)
}

func (h *PricingHandler) GetVenuePriceStats(c *gin.Context) {
	startDate := c.Query("start_date")
	endDate := c.Query("end_date")

	result, err := h.AnalysisSvc.GetVenuePriceStats(startDate, endDate)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"detail": "查询失败"})
		return
	}

	c.JSON(http.StatusOK, result)
}

// ---------- 上座率查询 ----------

func (h *PricingHandler) GetOccupancy(c *gin.Context) {
	venueID, _ := strconv.ParseUint(c.Param("venue_id"), 10, 32)
	date := c.Query("date")
	startHour, _ := strconv.Atoi(c.Query("start_hour"))
	endHour, _ := strconv.Atoi(c.Query("end_hour"))

	result, err := h.LockSvc.GetOccupancy(uint(venueID), date, startHour, endHour)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"detail": err.Error()})
		return
	}

	c.JSON(http.StatusOK, result)
}

func getUserID(c *gin.Context) uint {
	if user, exists := c.Get("user"); exists {
		if u, ok := user.(models.User); ok {
			return u.ID
		}
	}

	_ = auth.Middleware
	return 0
}

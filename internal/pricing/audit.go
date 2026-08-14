package pricing

import (
	"encoding/json"
	"time"

	"gorm.io/gorm"

	"venue-booking-admin/internal/models"
)

// AuditService 价格审计服务
type AuditService struct {
	db *gorm.DB
}

// NewAuditService 创建审计服务
func NewAuditService(db *gorm.DB) *AuditService {
	return &AuditService{db: db}
}

// LogAction 记录价格变更操作
func (s *AuditService) LogAction(venueID uint, action string, operatorID uint, operatorName string, before interface{}, after interface{}, ip string) error {
	var beforeContent, afterContent string

	if before != nil {
		b, _ := json.Marshal(before)
		beforeContent = string(b)
	}
	if after != nil {
		a, _ := json.Marshal(after)
		afterContent = string(a)
	}

	log := &models.PriceAuditLog{
		VenueID:       venueID,
		Action:        action,
		OperatorID:    operatorID,
		OperatorName:  operatorName,
		BeforeContent: beforeContent,
		AfterContent:  afterContent,
		IP:            ip,
		CreatedAt:     time.Now(),
	}

	return s.db.Create(log).Error
}

// ListAuditLogs 查询审计日志
func (s *AuditService) ListAuditLogs(venueID uint, action string, startTime, endTime time.Time, page, pageSize int) ([]models.PriceAuditLog, int64, error) {
	var logs []models.PriceAuditLog
	var total int64

	query := s.db.Model(&models.PriceAuditLog{})

	if venueID > 0 {
		query = query.Where("venue_id = ?", venueID)
	}
	if action != "" {
		query = query.Where("action = ?", action)
	}
	if !startTime.IsZero() {
		query = query.Where("created_at >= ?", startTime)
	}
	if !endTime.IsZero() {
		query = query.Where("created_at <= ?", endTime)
	}

	query.Count(&total)

	offset := (page - 1) * pageSize
	if offset < 0 {
		offset = 0
	}

	err := query.Order("created_at DESC").
		Offset(offset).
		Limit(pageSize).
		Find(&logs).Error

	return logs, total, err
}

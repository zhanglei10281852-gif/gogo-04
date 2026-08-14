package pricing

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"venue-booking-admin/internal/models"
)

// LockService 价格锁服务
type LockService struct {
	db            *gorm.DB
	engine        *Engine
	lockDuration  time.Duration
}

// NewLockService 创建价格锁服务
func NewLockService(db *gorm.DB, engine *Engine) *LockService {
	return &LockService{
		db:           db,
		engine:       engine,
		lockDuration: 15 * time.Minute,
	}
}

// LockPrice 锁定价格
func (s *LockService) LockPrice(query PriceQuery, ip string) (*models.PriceLock, error) {
	result, err := s.engine.CalculatePrice(query)
	if err != nil {
		return nil, err
	}

	lockKey := generateLockKey(query.VenueID, query.BookDate, query.StartHour, query.EndHour)

	var existingLock models.PriceLock
	err = s.db.Where("lock_key = ? AND expires_at > ?", lockKey, time.Now()).
		First(&existingLock).Error

	if err == nil {
		existingLock.ExpiresAt = time.Now().Add(s.lockDuration)
		s.db.Save(&existingLock)
		return &existingLock, nil
	}

	priceDetail, _ := json.Marshal(result)

	lock := &models.PriceLock{
		LockKey:      lockKey,
		VenueID:      query.VenueID,
		BookDate:     query.BookDate,
		StartHour:    query.StartHour,
		EndHour:      query.EndHour,
		LockedPrice:  result.FinalPrice,
		LockedAmount: result.TotalAmount,
		PriceDetail:  string(priceDetail),
		ExpiresAt:    time.Now().Add(s.lockDuration),
	}

	if err := s.db.Create(lock).Error; err != nil {
		return nil, err
	}

	return lock, nil
}

// GetLockedPrice 获取已锁定的价格
func (s *LockService) GetLockedPrice(lockKey string) (*models.PriceLock, error) {
	var lock models.PriceLock
	err := s.db.Where("lock_key = ? AND expires_at > ?", lockKey, time.Now()).
		First(&lock).Error
	if err != nil {
		return nil, errors.New("价格锁不存在或已过期")
	}
	return &lock, nil
}

// VerifyAndUseLock 验证并使用锁（关联预订）
func (s *LockService) VerifyAndUseLock(lockKey string, bookingID uint) (*models.PriceLock, error) {
	var lock models.PriceLock
	err := s.db.Where("lock_key = ? AND expires_at > ? AND booking_id IS NULL", lockKey, time.Now()).
		First(&lock).Error
	if err != nil {
		return nil, errors.New("价格锁不存在、已过期或已使用")
	}

	lock.BookingID = &bookingID
	lock.ExpiresAt = time.Now().Add(24 * time.Hour)

	if err := s.db.Save(&lock).Error; err != nil {
		return nil, err
	}

	return &lock, nil
}

// ReleaseLock 释放价格锁
func (s *LockService) ReleaseLock(lockKey string) error {
	return s.db.Where("lock_key = ?", lockKey).Delete(&models.PriceLock{}).Error
}

// CleanExpiredLocks 清理过期的锁
func (s *LockService) CleanExpiredLocks() (int64, error) {
	result := s.db.Where("expires_at < ?", time.Now()).Delete(&models.PriceLock{})
	return result.RowsAffected, result.Error
}

func generateLockKey(venueID uint, bookDate string, startHour, endHour int) string {
	raw := fmt.Sprintf("%d_%s_%d_%d_%d", venueID, bookDate, startHour, endHour, time.Now().UnixNano())
	hash := md5.Sum([]byte(raw))
	return hex.EncodeToString(hash[:])
}

// GetOccupancy 获取指定时段上座率
func (s *LockService) GetOccupancy(venueID uint, date string, startHour, endHour int) (*models.OccupancyRate, error) {
	var venue models.Venue
	if err := s.db.First(&venue, venueID).Error; err != nil {
		return nil, errors.New("场馆不存在")
	}

	var bookedCount int64
	s.db.Model(&models.Booking{}).
		Where("venue_id = ? AND book_date = ? AND status <> ?", venueID, date, "cancelled").
		Where("start_hour < ? AND end_hour > ?", endHour, startHour).
		Count(&bookedCount)

	rate := 0.0
	if venue.Capacity > 0 {
		rate = float64(bookedCount) / float64(venue.Capacity)
	}

	return &models.OccupancyRate{
		VenueID:  venueID,
		Date:     date,
		Hour:     startHour,
		Rate:     rate,
		Booked:   int(bookedCount),
		Capacity: venue.Capacity,
	}, nil
}

package pricing

import (
	"encoding/json"
	"errors"
	"time"

	"gorm.io/gorm"

	"venue-booking-admin/internal/models"
)

// Engine 定价引擎
type Engine struct {
	db *gorm.DB
}

// NewEngine 创建定价引擎
func NewEngine(db *gorm.DB) *Engine {
	return &Engine{db: db}
}

// PriceQuery 价格查询参数
type PriceQuery struct {
	VenueID     uint      `json:"venue_id"`
	BookDate    string    `json:"book_date"` // YYYY-MM-DD
	StartHour   int       `json:"start_hour"`
	EndHour     int       `json:"end_hour"`
	MemberLevel string    `json:"member_level"` // 会员等级
	Occupancy   *float64  `json:"occupancy"`    // 指定上座率（用于模拟），nil则实时计算
	QueryTime   time.Time `json:"query_time"`   // 查询时间，用于提前天数计算，默认为当前时间
}

// CalculatePrice 计算价格
func (e *Engine) CalculatePrice(query PriceQuery) (*models.PriceCalculationResult, error) {
	venue, err := e.getVenue(query.VenueID)
	if err != nil {
		return nil, err
	}

	bindings, err := e.getActiveBindings(query.VenueID)
	if err != nil {
		return nil, err
	}

	holiday, err := e.isHoliday(query.BookDate)
	if err != nil {
		return nil, err
	}

	var occupancyRate float64
	if query.Occupancy != nil {
		occupancyRate = *query.Occupancy
	} else {
		occupancyRate, err = e.calculateOccupancy(query.VenueID, query.BookDate, query.StartHour, query.EndHour)
		if err != nil {
			return nil, err
		}
	}

	queryTime := query.QueryTime
	if queryTime.IsZero() {
		queryTime = time.Now()
	}

	advanceDays, err := calculateAdvanceDays(query.BookDate, queryTime)
	if err != nil {
		return nil, err
	}

	weekday := getWeekday(query.BookDate)

	currentPrice := venue.HourlyPrice
	result := &models.PriceCalculationResult{
		BasePrice:  venue.HourlyPrice,
		AppliedRules: []models.AppliedRuleInfo{},
	}

	var minPrice, maxPrice float64

	for _, binding := range bindings {
		if binding.MinPrice > 0 && (minPrice == 0 || binding.MinPrice < minPrice) {
			minPrice = binding.MinPrice
		}
		if binding.MaxPrice > 0 && (maxPrice == 0 || binding.MaxPrice > maxPrice) {
			maxPrice = binding.MaxPrice
		}

		if binding.Rule == nil {
			continue
		}
		rule := binding.Rule
		if !rule.IsActive {
			continue
		}

		matched, err := e.matchRule(*rule, query, holiday, occupancyRate, advanceDays, weekday)
		if err != nil {
			continue
		}
		if !matched {
			continue
		}

		priceBefore := currentPrice
		newPrice := applyRule(currentPrice, *rule)

		result.AppliedRules = append(result.AppliedRules, models.AppliedRuleInfo{
			RuleID:      rule.ID,
			RuleName:    rule.Name,
			RuleType:    rule.RuleType,
			Operator:    rule.Operator,
			Value:       rule.Value,
			PriceBefore: priceBefore,
			PriceAfter:  newPrice,
		})

		currentPrice = newPrice
	}

	promotions, err := e.getActivePromotions(query.VenueID, query.BookDate, weekday)
	if err == nil {
		for _, promo := range promotions {
			priceBefore := currentPrice
			newPrice := applyPromotion(currentPrice, promo, float64(query.EndHour-query.StartHour))

			if newPrice != priceBefore {
				result.AppliedRules = append(result.AppliedRules, models.AppliedRuleInfo{
					RuleID:      uint(promo.ID),
					RuleName:    promo.Name,
					RuleType:    models.RuleType("promotion"),
					Operator:    models.OperatorMultiply,
					Value:       promo.DiscountRate,
					PriceBefore: priceBefore,
					PriceAfter:  newPrice,
				})
				currentPrice = newPrice
			}
		}
	}

	result.MinPrice = minPrice
	result.MaxPrice = maxPrice

	if minPrice > 0 && currentPrice < minPrice {
		currentPrice = minPrice
		result.IsMinLimited = true
	}
	if maxPrice > 0 && currentPrice > maxPrice {
		currentPrice = maxPrice
		result.IsMaxLimited = true
	}

	result.FinalPrice = roundPrice(currentPrice)
	hours := float64(query.EndHour - query.StartHour)
	result.TotalAmount = roundPrice(result.FinalPrice * hours)

	return result, nil
}

func (e *Engine) getVenue(venueID uint) (*models.Venue, error) {
	var venue models.Venue
	if err := e.db.First(&venue, venueID).Error; err != nil {
		return nil, errors.New("场馆不存在")
	}
	return &venue, nil
}

func (e *Engine) getActiveBindings(venueID uint) ([]models.VenuePricingBinding, error) {
	var bindings []models.VenuePricingBinding
	err := e.db.Preload("Rule").
		Where("venue_id = ? AND is_active = ?", venueID, true).
		Order("priority DESC, id ASC").
		Find(&bindings).Error
	return bindings, err
}

func (e *Engine) isHoliday(date string) (bool, error) {
	var holiday models.Holiday
	err := e.db.Where("date = ?", date).First(&holiday).Error
	if err == gorm.ErrRecordNotFound {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return !holiday.IsWorkday, nil
}

func (e *Engine) calculateOccupancy(venueID uint, date string, startHour, endHour int) (float64, error) {
	var venue models.Venue
	if err := e.db.First(&venue, venueID).Error; err != nil {
		return 0, err
	}

	var bookedCount int64
	e.db.Model(&models.Booking{}).
		Where("venue_id = ? AND book_date = ? AND status <> ?", venueID, date, "cancelled").
		Where("start_hour < ? AND end_hour > ?", endHour, startHour).
		Count(&bookedCount)

	if venue.Capacity <= 0 {
		return 0, nil
	}

	return float64(bookedCount) / float64(venue.Capacity), nil
}

func (e *Engine) matchRule(rule models.PricingRule, query PriceQuery, isHoliday bool, occupancyRate float64, advanceDays int, weekday int) (bool, error) {
	switch rule.RuleType {
	case models.RuleTypeBasePrice:
		return true, nil

	case models.RuleTypeTimeSlot:
		var config models.TimeSlotConfig
		if err := json.Unmarshal([]byte(rule.Config), &config); err != nil {
			return false, err
		}
		return query.StartHour >= config.StartHour && query.EndHour <= config.EndHour, nil

	case models.RuleTypeWeekday:
		var config models.WeekdayConfig
		if err := json.Unmarshal([]byte(rule.Config), &config); err != nil {
			return false, err
		}
		for _, wd := range config.Weekdays {
			if wd == weekday {
				return true, nil
			}
		}
		return false, nil

	case models.RuleTypeHoliday:
		var config models.HolidayConfig
		if err := json.Unmarshal([]byte(rule.Config), &config); err != nil {
			return false, err
		}
		return config.IsHoliday == isHoliday, nil

	case models.RuleTypeAdvanceBooking:
		var config models.AdvanceBookingConfig
		if err := json.Unmarshal([]byte(rule.Config), &config); err != nil {
			return false, err
		}
		return advanceDays >= config.MinDays && advanceDays <= config.MaxDays, nil

	case models.RuleTypeOccupancy:
		var config models.OccupancyConfig
		if err := json.Unmarshal([]byte(rule.Config), &config); err != nil {
			return false, err
		}
		return occupancyRate >= config.MinRate && occupancyRate < config.MaxRate, nil

	case models.RuleTypeMemberDiscount:
		var config models.MemberDiscountConfig
		if err := json.Unmarshal([]byte(rule.Config), &config); err != nil {
			return false, err
		}
		return query.MemberLevel == config.MemberLevel, nil

	default:
		return false, nil
	}
}

func applyRule(currentPrice float64, rule models.PricingRule) float64 {
	switch rule.Operator {
	case models.OperatorMultiply:
		return currentPrice * rule.Value
	case models.OperatorAdd:
		return currentPrice + rule.Value
	case models.OperatorSubtract:
		result := currentPrice - rule.Value
		if result < 0 {
			return 0
		}
		return result
	case models.OperatorSet:
		return rule.Value
	default:
		return currentPrice
	}
}

func applyPromotion(currentPrice float64, promo models.Promotion, hours float64) float64 {
	switch promo.PromotionType {
	case models.PromotionTypeDiscount:
		return currentPrice * promo.DiscountRate
	case models.PromotionTypeFullReduce:
		total := currentPrice * hours
		if total >= promo.FullAmount {
			reducePerHour := promo.ReduceAmount / hours
			return currentPrice - reducePerHour
		}
		return currentPrice
	default:
		return currentPrice
	}
}

func (e *Engine) getActivePromotions(venueID uint, date string, weekday int) ([]models.Promotion, error) {
	var promotions []models.Promotion
	now := time.Now()

	query := e.db.Where("is_active = ?", true).
		Where("start_time <= ? AND end_time >= ?", now, now).
		Order("id ASC")

	err := query.Find(&promotions).Error
	if err != nil {
		return nil, err
	}

	var result []models.Promotion
	for _, promo := range promotions {
		venueIDs := parseJSONUintArray(promo.VenueIDs)
		if len(venueIDs) > 0 {
			found := false
			for _, vid := range venueIDs {
				if vid == venueID {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}

		weekdays := parseJSONIntArray(promo.Weekdays)
		if len(weekdays) > 0 {
			found := false
			for _, wd := range weekdays {
				if wd == weekday {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}

		result = append(result, promo)
	}

	return result, nil
}

func calculateAdvanceDays(bookDate string, now time.Time) (int, error) {
	bookTime, err := time.Parse("2006-01-02", bookDate)
	if err != nil {
		return 0, err
	}
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	diff := bookTime.Sub(today)
	return int(diff.Hours() / 24), nil
}

func getWeekday(date string) int {
	t, err := time.Parse("2006-01-02", date)
	if err != nil {
		return -1
	}
	return int(t.Weekday())
}

func roundPrice(price float64) float64 {
	return float64(int(price*100+0.5)) / 100
}

func parseJSONUintArray(s string) []uint {
	var result []uint
	if s == "" {
		return result
	}
	json.Unmarshal([]byte(s), &result)
	return result
}

func parseJSONIntArray(s string) []int {
	var result []int
	if s == "" {
		return result
	}
	json.Unmarshal([]byte(s), &result)
	return result
}

// SimulateDayPrice 模拟一天的价格曲线
func (e *Engine) SimulateDayPrice(venueID uint, date string, occupancy float64, memberLevel string) ([]HourPrice, error) {
	venue, err := e.getVenue(venueID)
	if err != nil {
		return nil, err
	}

	var result []HourPrice
	for hour := venue.OpenHour; hour < venue.CloseHour; hour++ {
		priceResult, err := e.CalculatePrice(PriceQuery{
			VenueID:     venueID,
			BookDate:    date,
			StartHour:   hour,
			EndHour:     hour + 1,
			MemberLevel: memberLevel,
			Occupancy:   &occupancy,
		})
		if err != nil {
			continue
		}

		result = append(result, HourPrice{
			Hour:       hour,
			Price:      priceResult.FinalPrice,
			BasePrice:  priceResult.BasePrice,
			IsMinLimit: priceResult.IsMinLimited,
			IsMaxLimit: priceResult.IsMaxLimited,
		})
	}

	return result, nil
}

// HourPrice 小时价格
type HourPrice struct {
	Hour       int     `json:"hour"`
	Price      float64 `json:"price"`
	BasePrice  float64 `json:"base_price"`
	IsMinLimit bool    `json:"is_min_limit"`
	IsMaxLimit bool    `json:"is_max_limit"`
}

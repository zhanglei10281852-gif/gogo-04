package pricing

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"gorm.io/driver/sqlite"
	_ "modernc.org/sqlite"
	"gorm.io/gorm"

	"venue-booking-admin/internal/models"
)

func setupTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Dialector{DriverName: "sqlite", DSN: ":memory:"}, &gorm.Config{})
	assert.NoError(t, err)

	db.AutoMigrate(
		&models.Venue{},
		&models.Booking{},
		&models.PricingRule{},
		&models.VenuePricingBinding{},
		&models.Holiday{},
		&models.Promotion{},
		&models.PriceLock{},
		&models.GrayRelease{},
		&models.PriceAuditLog{},
	)

	return db
}

func pUint(v uint) *uint {
	return &v
}

func TestApplyRule(t *testing.T) {
	tests := []struct {
		name       string
		current    float64
		operator   models.RuleOperator
		value      float64
		expected   float64
	}{
		{"multiply 1.5", 100, models.OperatorMultiply, 1.5, 150},
		{"multiply 0.8", 100, models.OperatorMultiply, 0.8, 80},
		{"add 50", 100, models.OperatorAdd, 50, 150},
		{"subtract 30", 100, models.OperatorSubtract, 30, 70},
		{"subtract negative floor", 20, models.OperatorSubtract, 30, 0},
		{"set price", 100, models.OperatorSet, 120, 120},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rule := models.PricingRule{
				Operator: tt.operator,
				Value:    tt.value,
			}
			result := applyRule(tt.current, rule)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestRoundPrice(t *testing.T) {
	assert.Equal(t, 100.0, roundPrice(100.0))
	assert.Equal(t, 100.55, roundPrice(100.545))
	assert.Equal(t, 100.56, roundPrice(100.555))
}

func TestCalculateAdvanceDays(t *testing.T) {
	now, _ := time.Parse("2006-01-02", "2026-06-20")

	tests := []struct {
		name     string
		bookDate string
		expected int
	}{
		{"same day", "2026-06-20", 0},
		{"next day", "2026-06-21", 1},
		{"in 7 days", "2026-06-27", 7},
		{"in 30 days", "2026-07-20", 30},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			days, err := calculateAdvanceDays(tt.bookDate, now)
			assert.NoError(t, err)
			assert.Equal(t, tt.expected, days)
		})
	}
}

func TestGetWeekday(t *testing.T) {
	assert.Equal(t, 6, getWeekday("2026-06-20")) // Saturday
	assert.Equal(t, 0, getWeekday("2026-06-21")) // Sunday
	assert.Equal(t, 1, getWeekday("2026-06-22")) // Monday
}

func TestBasePriceWithDB(t *testing.T) {
	db := setupTestDB(t)
	engine := NewEngine(db)

	venue := models.Venue{
		Name: "测试场馆", SportType: "basketball",
		Capacity: 100, HourlyPrice: 100,
		OpenHour: 8, CloseHour: 22, Status: "open",
	}
	db.Create(&venue)

	result, err := engine.CalculatePrice(PriceQuery{
		VenueID:   venue.ID,
		BookDate:  "2026-06-23",
		StartHour: 10,
		EndHour:   12,
	})

	assert.NoError(t, err)
	assert.Equal(t, 100.0, result.BasePrice)
	assert.Equal(t, 100.0, result.FinalPrice)
	assert.Equal(t, 200.0, result.TotalAmount)
}

func TestTimeSlotRuleWithDB(t *testing.T) {
	db := setupTestDB(t)
	engine := NewEngine(db)

	venue := models.Venue{
		Name: "测试场馆", SportType: "basketball",
		Capacity: 100, HourlyPrice: 100,
		OpenHour: 8, CloseHour: 22, Status: "open",
	}
	db.Create(&venue)

	eveningConfig, _ := json.Marshal(models.TimeSlotConfig{StartHour: 18, EndHour: 22})
	rule := models.PricingRule{
		Name: "晚场加价", RuleType: models.RuleTypeTimeSlot,
		Operator: models.OperatorMultiply, Value: 1.5,
		IsActive: true, Config: string(eveningConfig),
	}
	db.Create(&rule)

	binding := models.VenuePricingBinding{
		VenueID: venue.ID, RuleID: pUint(rule.ID),
		Priority: 100, IsActive: true,
	}
	db.Create(&binding)

	result, err := engine.CalculatePrice(PriceQuery{
		VenueID:   venue.ID,
		BookDate:  "2026-06-23",
		StartHour: 19,
		EndHour:   21,
	})

	assert.NoError(t, err)
	assert.Equal(t, 150.0, result.FinalPrice)
	assert.Equal(t, 300.0, result.TotalAmount)
	assert.Len(t, result.AppliedRules, 1)
	assert.Equal(t, "晚场加价", result.AppliedRules[0].RuleName)
}

func TestWeekdayRuleWithDB(t *testing.T) {
	db := setupTestDB(t)
	engine := NewEngine(db)

	venue := models.Venue{
		Name: "测试场馆", SportType: "basketball",
		Capacity: 100, HourlyPrice: 100,
		OpenHour: 8, CloseHour: 22, Status: "open",
	}
	db.Create(&venue)

	weekdayConfig, _ := json.Marshal(models.WeekdayConfig{Weekdays: []int{1, 2, 3, 4, 5}})
	rule := models.PricingRule{
		Name: "工作日折扣", RuleType: models.RuleTypeWeekday,
		Operator: models.OperatorMultiply, Value: 0.8,
		IsActive: true, Config: string(weekdayConfig),
	}
	db.Create(&rule)

	binding := models.VenuePricingBinding{
		VenueID: venue.ID, RuleID: pUint(rule.ID),
		Priority: 80, IsActive: true,
	}
	db.Create(&binding)

	result, err := engine.CalculatePrice(PriceQuery{
		VenueID:   venue.ID,
		BookDate:  "2026-06-23",
		StartHour: 10,
		EndHour:   11,
	})

	assert.NoError(t, err)
	assert.Equal(t, 80.0, result.FinalPrice)
}

func TestHolidayRuleWithDB(t *testing.T) {
	db := setupTestDB(t)
	engine := NewEngine(db)

	venue := models.Venue{
		Name: "测试场馆", SportType: "basketball",
		Capacity: 100, HourlyPrice: 100,
		OpenHour: 8, CloseHour: 22, Status: "open",
	}
	db.Create(&venue)

	holiday := models.Holiday{Date: "2026-10-01", Name: "国庆节"}
	db.Create(&holiday)

	holidayConfig, _ := json.Marshal(models.HolidayConfig{IsHoliday: true})
	rule := models.PricingRule{
		Name: "节假日加价", RuleType: models.RuleTypeHoliday,
		Operator: models.OperatorMultiply, Value: 1.3,
		IsActive: true, Config: string(holidayConfig),
	}
	db.Create(&rule)

	binding := models.VenuePricingBinding{
		VenueID: venue.ID, RuleID: pUint(rule.ID),
		Priority: 90, IsActive: true,
	}
	db.Create(&binding)

	result, err := engine.CalculatePrice(PriceQuery{
		VenueID:   venue.ID,
		BookDate:  "2026-10-01",
		StartHour: 10,
		EndHour:   11,
	})

	assert.NoError(t, err)
	assert.Equal(t, 130.0, result.FinalPrice)
}

func TestMinMaxPriceWithDB(t *testing.T) {
	db := setupTestDB(t)
	engine := NewEngine(db)

	venue := models.Venue{
		Name: "测试场馆", SportType: "basketball",
		Capacity: 100, HourlyPrice: 100,
		OpenHour: 8, CloseHour: 22, Status: "open",
	}
	db.Create(&venue)

	morningConfig, _ := json.Marshal(models.TimeSlotConfig{StartHour: 6, EndHour: 12})
	rule := models.PricingRule{
		Name: "早场5折", RuleType: models.RuleTypeTimeSlot,
		Operator: models.OperatorMultiply, Value: 0.5,
		IsActive: true, Config: string(morningConfig),
	}
	db.Create(&rule)

	binding := models.VenuePricingBinding{
		VenueID: venue.ID, RuleID: pUint(rule.ID),
		Priority: 90, IsActive: true,
		MinPrice: 60, MaxPrice: 0,
	}
	db.Create(&binding)

	result, err := engine.CalculatePrice(PriceQuery{
		VenueID:   venue.ID,
		BookDate:  "2026-06-23",
		StartHour: 9,
		EndHour:   10,
	})

	assert.NoError(t, err)
	assert.Equal(t, 60.0, result.FinalPrice)
	assert.True(t, result.IsMinLimited)
}

func TestAdvanceBookingRuleWithDB(t *testing.T) {
	db := setupTestDB(t)
	engine := NewEngine(db)

	venue := models.Venue{
		Name: "测试场馆", SportType: "basketball",
		Capacity: 100, HourlyPrice: 100,
		OpenHour: 8, CloseHour: 22, Status: "open",
	}
	db.Create(&venue)

	advanceConfig, _ := json.Marshal(models.AdvanceBookingConfig{MinDays: 7, MaxDays: 30})
	rule := models.PricingRule{
		Name: "提前7天8折", RuleType: models.RuleTypeAdvanceBooking,
		Operator: models.OperatorMultiply, Value: 0.8,
		IsActive: true, Config: string(advanceConfig),
	}
	db.Create(&rule)

	binding := models.VenuePricingBinding{
		VenueID: venue.ID, RuleID: pUint(rule.ID),
		Priority: 70, IsActive: true,
	}
	db.Create(&binding)

	queryTime, _ := time.Parse("2006-01-02", "2026-06-20")
	result, err := engine.CalculatePrice(PriceQuery{
		VenueID:   venue.ID,
		BookDate:  "2026-06-30",
		StartHour: 10,
		EndHour:   11,
		QueryTime: queryTime,
	})

	assert.NoError(t, err)
	assert.Equal(t, 80.0, result.FinalPrice)
}

func TestMemberDiscountWithDB(t *testing.T) {
	db := setupTestDB(t)
	engine := NewEngine(db)

	venue := models.Venue{
		Name: "测试场馆", SportType: "basketball",
		Capacity: 100, HourlyPrice: 100,
		OpenHour: 8, CloseHour: 22, Status: "open",
	}
	db.Create(&venue)

	vipConfig, _ := json.Marshal(models.MemberDiscountConfig{MemberLevel: "vip"})
	rule := models.PricingRule{
		Name: "VIP8折", RuleType: models.RuleTypeMemberDiscount,
		Operator: models.OperatorMultiply, Value: 0.8,
		IsActive: true, Config: string(vipConfig),
	}
	db.Create(&rule)

	binding := models.VenuePricingBinding{
		VenueID: venue.ID, RuleID: pUint(rule.ID),
		Priority: 110, IsActive: true,
	}
	db.Create(&binding)

	result, err := engine.CalculatePrice(PriceQuery{
		VenueID:     venue.ID,
		BookDate:    "2026-06-23",
		StartHour:   10,
		EndHour:     11,
		MemberLevel: "vip",
	})

	assert.NoError(t, err)
	assert.Equal(t, 80.0, result.FinalPrice)
}

func TestMultipleRulesWithDB(t *testing.T) {
	db := setupTestDB(t)
	engine := NewEngine(db)

	venue := models.Venue{
		Name: "测试场馆", SportType: "basketball",
		Capacity: 100, HourlyPrice: 100,
		OpenHour: 8, CloseHour: 22, Status: "open",
	}
	db.Create(&venue)

	weekdayConfig, _ := json.Marshal(models.WeekdayConfig{Weekdays: []int{2}})
	weekdayRule := models.PricingRule{
		Name: "周二特惠", RuleType: models.RuleTypeWeekday,
		Operator: models.OperatorMultiply, Value: 0.9,
		IsActive: true, Config: string(weekdayConfig),
	}
	db.Create(&weekdayRule)

	morningConfig, _ := json.Marshal(models.TimeSlotConfig{StartHour: 8, EndHour: 12})
	morningRule := models.PricingRule{
		Name: "早场优惠", RuleType: models.RuleTypeTimeSlot,
		Operator: models.OperatorMultiply, Value: 0.8,
		IsActive: true, Config: string(morningConfig),
	}
	db.Create(&morningRule)

	db.Create(&models.VenuePricingBinding{
		VenueID: venue.ID, RuleID: pUint(weekdayRule.ID),
		Priority: 80, IsActive: true,
	})
	db.Create(&models.VenuePricingBinding{
		VenueID: venue.ID, RuleID: pUint(morningRule.ID),
		Priority: 90, IsActive: true,
	})

	result, err := engine.CalculatePrice(PriceQuery{
		VenueID:   venue.ID,
		BookDate:  "2026-06-23",
		StartHour: 10,
		EndHour:   11,
	})

	assert.NoError(t, err)
	assert.Equal(t, 72.0, result.FinalPrice)
	assert.Len(t, result.AppliedRules, 2)
}

func TestPriceLockWithDB(t *testing.T) {
	db := setupTestDB(t)
	engine := NewEngine(db)
	lockSvc := NewLockService(db, engine)

	venue := models.Venue{
		Name: "测试场馆", SportType: "basketball",
		Capacity: 100, HourlyPrice: 100,
		OpenHour: 8, CloseHour: 22, Status: "open",
	}
	db.Create(&venue)

	lock, err := lockSvc.LockPrice(PriceQuery{
		VenueID:   venue.ID,
		BookDate:  "2026-06-25",
		StartHour: 10,
		EndHour:   12,
	}, "127.0.0.1")

	assert.NoError(t, err)
	assert.NotNil(t, lock)
	assert.Equal(t, 100.0, lock.LockedPrice)
	assert.Equal(t, 200.0, lock.LockedAmount)
	assert.NotEmpty(t, lock.LockKey)

	retrieved, err := lockSvc.GetLockedPrice(lock.LockKey)
	assert.NoError(t, err)
	assert.Equal(t, lock.ID, retrieved.ID)
}

func TestSimulateDayPriceWithDB(t *testing.T) {
	db := setupTestDB(t)
	engine := NewEngine(db)

	venue := models.Venue{
		Name: "测试场馆", SportType: "basketball",
		Capacity: 100, HourlyPrice: 100,
		OpenHour: 8, CloseHour: 22, Status: "open",
	}
	db.Create(&venue)

	morningConfig, _ := json.Marshal(models.TimeSlotConfig{StartHour: 6, EndHour: 12})
	morningRule := models.PricingRule{
		Name: "早场8折", RuleType: models.RuleTypeTimeSlot,
		Operator: models.OperatorMultiply, Value: 0.8,
		IsActive: true, Config: string(morningConfig),
	}
	db.Create(&morningRule)

	eveningConfig, _ := json.Marshal(models.TimeSlotConfig{StartHour: 18, EndHour: 22})
	eveningRule := models.PricingRule{
		Name: "晚场1.5倍", RuleType: models.RuleTypeTimeSlot,
		Operator: models.OperatorMultiply, Value: 1.5,
		IsActive: true, Config: string(eveningConfig),
	}
	db.Create(&eveningRule)

	db.Create(&models.VenuePricingBinding{
		VenueID: venue.ID, RuleID: pUint(morningRule.ID),
		Priority: 90, IsActive: true,
	})
	db.Create(&models.VenuePricingBinding{
		VenueID: venue.ID, RuleID: pUint(eveningRule.ID),
		Priority: 100, IsActive: true,
	})

	hourPrices, err := engine.SimulateDayPrice(venue.ID, "2026-06-23", 0.5, "")
	assert.NoError(t, err)
	assert.Len(t, hourPrices, 14)

	for _, hp := range hourPrices {
		if hp.Hour >= 8 && hp.Hour < 12 {
			assert.Equal(t, 80.0, hp.Price)
		} else if hp.Hour >= 18 && hp.Hour < 22 {
			assert.Equal(t, 150.0, hp.Price)
		} else {
			assert.Equal(t, 100.0, hp.Price)
		}
	}
}

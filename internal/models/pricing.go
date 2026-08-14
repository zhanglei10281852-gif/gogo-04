package models

import (
	"time"
)

// RuleType 定价规则类型
type RuleType string

const (
	RuleTypeTimeSlot       RuleType = "time_slot"        // 按时段定价
	RuleTypeWeekday        RuleType = "weekday"          // 按星期定价
	RuleTypeHoliday        RuleType = "holiday"          // 节假日加价
	RuleTypeAdvanceBooking RuleType = "advance_booking"  // 提前预订折扣/加价
	RuleTypeOccupancy      RuleType = "occupancy"        // 上座率动态定价
	RuleTypeMemberDiscount RuleType = "member_discount"  // 会员折扣
	RuleTypeBasePrice      RuleType = "base_price"       // 基础价格
)

// RuleOperator 规则运算符
type RuleOperator string

const (
	OperatorMultiply  RuleOperator = "multiply"   // 乘以倍率
	OperatorAdd       RuleOperator = "add"        // 加上固定金额
	OperatorSet       RuleOperator = "set"        // 设置为固定价格
	OperatorSubtract  RuleOperator = "subtract"   // 减去固定金额
)

// PricingRule 定价规则
type PricingRule struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	Name        string    `gorm:"size:128" json:"name"`
	RuleType    RuleType  `gorm:"size:32;index" json:"rule_type"`
	Description string    `gorm:"size:512" json:"description"`
	Operator    RuleOperator `gorm:"size:16" json:"operator"`
	Value       float64   `json:"value"`                     // 规则值：倍率或金额
	IsActive    bool      `gorm:"default:true" json:"is_active"`

	// 通用条件字段（JSON存储具体条件）
	Config string `gorm:"type:text" json:"config"` // 规则配置，如时段范围、星期几等

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TimeSlotConfig 时段规则配置
type TimeSlotConfig struct {
	StartHour int `json:"start_hour"`
	EndHour   int `json:"end_hour"`
}

// WeekdayConfig 星期规则配置
type WeekdayConfig struct {
	Weekdays []int `json:"weekdays"` // 0=周日, 1=周一, ..., 6=周六
}

// HolidayConfig 节假日规则配置
type HolidayConfig struct {
	IsHoliday bool `json:"is_holiday"` // true=节假日适用, false=工作日适用
}

// AdvanceBookingConfig 提前预订配置
type AdvanceBookingConfig struct {
	MinDays int `json:"min_days"` // 最小提前天数
	MaxDays int `json:"max_days"` // 最大提前天数
}

// OccupancyConfig 上座率配置
type OccupancyConfig struct {
	MinRate float64 `json:"min_rate"` // 最小上座率 (0-1)
	MaxRate float64 `json:"max_rate"` // 最大上座率 (0-1)
}

// MemberDiscountConfig 会员折扣配置
type MemberDiscountConfig struct {
	MemberLevel string `json:"member_level"` // 会员等级
}

// VenuePricingBinding 场馆定价规则绑定
type VenuePricingBinding struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	VenueID    uint      `gorm:"index:idx_venue_priority" json:"venue_id"`
	RuleID     *uint     `gorm:"index" json:"rule_id"` // 可空，仅用于设置保底封顶时不需要规则
	Priority   int       `gorm:"index:idx_venue_priority" json:"priority"` // 优先级，数字越大优先级越高
	IsActive   bool      `gorm:"default:true" json:"is_active"`

	// 保底封顶（绑定级别）
	MinPrice   float64   `json:"min_price"` // 保底价（每小时），0表示不限制
	MaxPrice   float64   `json:"max_price"` // 封顶价（每小时），0表示不限制

	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`

	Rule *PricingRule `gorm:"foreignKey:RuleID" json:"rule,omitempty"`
}

// Holiday 节假日
type Holiday struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	Date        string    `gorm:"size:10;uniqueIndex" json:"date"` // YYYY-MM-DD
	Name        string    `gorm:"size:64" json:"name"`
	IsWorkday   bool      `gorm:"default:false" json:"is_workday"` // 是否为调休工作日
	CreatedAt   time.Time `json:"created_at"`
}

// PriceAuditLog 价格变更审计日志
type PriceAuditLog struct {
	ID            uint      `gorm:"primaryKey" json:"id"`
	VenueID       uint      `gorm:"index" json:"venue_id"`
	Action        string    `gorm:"size:32" json:"action"` // create_rule / update_rule / delete_rule / bind_rule / unbind_rule / update_binding
	OperatorID    uint      `json:"operator_id"`
	OperatorName  string    `gorm:"size:64" json:"operator_name"`
	BeforeContent string    `gorm:"type:text" json:"before_content"`
	AfterContent  string    `gorm:"type:text" json:"after_content"`
	IP            string    `gorm:"size:64" json:"ip"`
	CreatedAt     time.Time `gorm:"index" json:"created_at"`
}

// PriceLock 下单锁价记录
type PriceLock struct {
	ID            uint      `gorm:"primaryKey" json:"id"`
	LockKey       string    `gorm:"size:64;uniqueIndex" json:"lock_key"` // 锁的唯一标识
	VenueID       uint      `gorm:"index" json:"venue_id"`
	BookDate      string    `gorm:"size:10;index" json:"book_date"`
	StartHour     int       `json:"start_hour"`
	EndHour       int       `json:"end_hour"`
	LockedPrice   float64   `json:"locked_price"`   // 锁定的单价（每小时）
	LockedAmount  float64   `json:"locked_amount"`  // 锁定的总金额
	PriceDetail   string    `gorm:"type:text" json:"price_detail"` // 价格明细JSON
	ExpiresAt     time.Time `gorm:"index" json:"expires_at"`
	BookingID     *uint     `gorm:"index" json:"booking_id,omitempty"` // 关联的预订ID
	CreatedAt     time.Time `json:"created_at"`
}

// PromotionType 促销活动类型
type PromotionType string

const (
	PromotionTypeDiscount   PromotionType = "discount"   // 限时折扣
	PromotionTypeFullReduce PromotionType = "full_reduce" // 满减
)

// Promotion 促销活动
type Promotion struct {
	ID            uint      `gorm:"primaryKey" json:"id"`
	Name          string    `gorm:"size:128" json:"name"`
	PromotionType PromotionType `gorm:"size:32;index" json:"promotion_type"`
	Description   string    `gorm:"size:512" json:"description"`

	// 时间范围
	StartTime     time.Time `json:"start_time"`
	EndTime       time.Time `json:"end_time"`

	// 适用范围
	VenueIDs      string    `gorm:"type:text" json:"venue_ids"` // JSON数组，空表示全部
	Weekdays      string    `gorm:"type:text" json:"weekdays"`  // JSON数组，适用星期

	// 折扣配置
	DiscountRate  float64   `json:"discount_rate"` // 折扣率，0.8表示8折
	FullAmount    float64   `json:"full_amount"`   // 满减门槛
	ReduceAmount  float64   `json:"reduce_amount"` // 满减金额

	IsActive      bool      `gorm:"default:true" json:"is_active"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// GrayRelease 灰度发布配置
type GrayRelease struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	Name        string    `gorm:"size:128" json:"name"`
	Description string    `gorm:"size:512" json:"description"`
	Strategy    string    `gorm:"size:32" json:"strategy"` // venue_list / percentage
	VenueList   string    `gorm:"type:text" json:"venue_list"` // JSON数组，场馆ID列表
	Percentage  int       `json:"percentage"` // 百分比策略，0-100
	IsActive    bool      `gorm:"default:false" json:"is_active"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// PriceCalculationResult 价格计算结果
type PriceCalculationResult struct {
	BasePrice    float64           `json:"base_price"`    // 基础价格
	FinalPrice   float64           `json:"final_price"`   // 最终价格（每小时）
	TotalAmount  float64           `json:"total_amount"`  // 总金额
	AppliedRules []AppliedRuleInfo `json:"applied_rules"` // 应用的规则
	MinPrice     float64           `json:"min_price"`     // 保底价
	MaxPrice     float64           `json:"max_price"`     // 封顶价
	IsMinLimited bool              `json:"is_min_limited"` // 是否触底
	IsMaxLimited bool              `json:"is_max_limited"` // 是否触顶
}

// AppliedRuleInfo 应用的规则信息
type AppliedRuleInfo struct {
	RuleID      uint     `json:"rule_id"`
	RuleName    string   `json:"rule_name"`
	RuleType    RuleType `json:"rule_type"`
	Operator    RuleOperator `json:"operator"`
	Value       float64  `json:"value"`
	PriceBefore float64  `json:"price_before"`
	PriceAfter  float64  `json:"price_after"`
}

// OccupancyRate 上座率信息
type OccupancyRate struct {
	VenueID   uint    `json:"venue_id"`
	Date      string  `json:"date"`
	Hour      int     `json:"hour"`
	Rate      float64 `json:"rate"` // 0-1
	Booked    int     `json:"booked"`
	Capacity  int     `json:"capacity"`
}

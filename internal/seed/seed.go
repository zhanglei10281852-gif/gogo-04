package seed

import (
	"encoding/json"
	"log"
	"time"

	"gorm.io/gorm"

	"venue-booking-admin/internal/auth"
	"venue-booking-admin/internal/models"
)

// Run 初始化内置管理员与种子业务数据（幂等）。
func Run(database *gorm.DB, adminUser, adminPass string) error {
	var count int64
	database.Model(&models.User{}).Where("username = ?", adminUser).Count(&count)
	if count == 0 {
		hash, err := auth.HashPassword(adminPass)
		if err != nil {
			return err
		}
		database.Create(&models.User{Username: adminUser, PasswordHash: hash, DisplayName: "平台管理员"})
		log.Println("已创建管理员账号")
	}

	var venueCount int64
	database.Model(&models.Venue{}).Count(&venueCount)
	if venueCount == 0 {
		venues := []models.Venue{
			{Name: "城北全民健身中心篮球馆", SportType: "basketball", Capacity: 200, HourlyPrice: 160, OpenHour: 8, CloseHour: 22, Status: "open"},
			{Name: "奥体中心游泳馆", SportType: "swimming", Capacity: 400, HourlyPrice: 80, OpenHour: 6, CloseHour: 21, Status: "open"},
			{Name: "市民广场羽毛球馆", SportType: "badminton", Capacity: 60, HourlyPrice: 50, OpenHour: 9, CloseHour: 22, Status: "maintenance"},
			{Name: "滨江足球公园", SportType: "football", Capacity: 500, HourlyPrice: 300, OpenHour: 8, CloseHour: 20, Status: "open"},
		}
		if err := database.Create(&venues).Error; err != nil {
			return err
		}

		bookings := []models.Booking{
			{VenueID: venues[0].ID, CustomerName: "陈刚", Phone: "13700001111", BookDate: "2026-06-20", StartHour: 18, EndHour: 20, Amount: 480, Status: "booked"},
			{VenueID: venues[0].ID, CustomerName: "周敏", Phone: "13700002222", BookDate: "2026-06-20", StartHour: 20, EndHour: 21, Amount: 240, Status: "booked"},
			{VenueID: venues[1].ID, CustomerName: "黄磊", Phone: "13700003333", BookDate: "2026-06-21", StartHour: 7, EndHour: 9, Amount: 160, Status: "completed"},
			{VenueID: venues[3].ID, CustomerName: "吴静", Phone: "13700004444", BookDate: "2026-06-22", StartHour: 15, EndHour: 17, Amount: 600, Status: "cancelled"},
			{VenueID: venues[0].ID, CustomerName: "李华", Phone: "13700005555", BookDate: "2026-06-21", StartHour: 10, EndHour: 12, Amount: 288, Status: "booked"},
			{VenueID: venues[0].ID, CustomerName: "王强", Phone: "13700006666", BookDate: "2026-06-22", StartHour: 19, EndHour: 21, Amount: 576, Status: "booked"},
			{VenueID: venues[1].ID, CustomerName: "张芳", Phone: "13700007777", BookDate: "2026-06-20", StartHour: 14, EndHour: 16, Amount: 200, Status: "booked"},
			{VenueID: venues[2].ID, CustomerName: "刘洋", Phone: "13700008888", BookDate: "2026-06-23", StartHour: 15, EndHour: 17, Amount: 110, Status: "booked"},
		}
		if err := database.Create(&bookings).Error; err != nil {
			return err
		}
	}

	var ruleCount int64
	database.Model(&models.PricingRule{}).Count(&ruleCount)
	if ruleCount == 0 {
		rules := createSeedPricingRules()
		if err := database.Create(&rules).Error; err != nil {
			return err
		}

		var venues []models.Venue
		database.Find(&venues)

		bindings := createSeedBindings(rules, venues)
		if err := database.Create(&bindings).Error; err != nil {
			return err
		}

		log.Println("已创建定价规则与绑定")
	}

	var holidayCount int64
	database.Model(&models.Holiday{}).Count(&holidayCount)
	if holidayCount == 0 {
		holidays := createSeedHolidays()
		if err := database.Create(&holidays).Error; err != nil {
			return err
		}
		log.Println("已创建节假日数据")
	}

	var promotionCount int64
	database.Model(&models.Promotion{}).Count(&promotionCount)
	if promotionCount == 0 {
		promotions := createSeedPromotions()
		if err := database.Create(&promotions).Error; err != nil {
			return err
		}
		log.Println("已创建促销活动数据")
	}

	var grayCount int64
	database.Model(&models.GrayRelease{}).Count(&grayCount)
	if grayCount == 0 {
		var venues []models.Venue
		database.Where("sport_type = ?", "basketball").Find(&venues)

		venueIDs := make([]uint, 0)
		for _, v := range venues {
			venueIDs = append(venueIDs, v.ID)
		}
		venueListJSON, _ := json.Marshal(venueIDs)

		gray := models.GrayRelease{
			Name:        "动态定价灰度测试",
			Description: "在篮球馆试点动态定价规则，验证效果后全量上线",
			Strategy:    "venue_list",
			VenueList:   string(venueListJSON),
			Percentage:  100,
			IsActive:    false,
		}
		if err := database.Create(&gray).Error; err != nil {
			return err
		}
		log.Println("已创建灰度发布配置")
	}

	log.Println("种子数据初始化完成")
	return nil
}

func createSeedPricingRules() []models.PricingRule {
	morningConfig, _ := json.Marshal(models.TimeSlotConfig{StartHour: 6, EndHour: 12})
	afternoonConfig, _ := json.Marshal(models.TimeSlotConfig{StartHour: 12, EndHour: 18})
	eveningConfig, _ := json.Marshal(models.TimeSlotConfig{StartHour: 18, EndHour: 22})

	weekdayConfig, _ := json.Marshal(models.WeekdayConfig{Weekdays: []int{1, 2, 3, 4, 5}})
	weekendConfig, _ := json.Marshal(models.WeekdayConfig{Weekdays: []int{0, 6}})

	holidayConfig, _ := json.Marshal(models.HolidayConfig{IsHoliday: true})

	advance7Config, _ := json.Marshal(models.AdvanceBookingConfig{MinDays: 7, MaxDays: 30})
	advance3Config, _ := json.Marshal(models.AdvanceBookingConfig{MinDays: 3, MaxDays: 6})
	advance1Config, _ := json.Marshal(models.AdvanceBookingConfig{MinDays: 0, MaxDays: 2})

	lowOccConfig, _ := json.Marshal(models.OccupancyConfig{MinRate: 0, MaxRate: 0.3})
	midOccConfig, _ := json.Marshal(models.OccupancyConfig{MinRate: 0.3, MaxRate: 0.7})
	highOccConfig, _ := json.Marshal(models.OccupancyConfig{MinRate: 0.7, MaxRate: 1.0})

	vipConfig, _ := json.Marshal(models.MemberDiscountConfig{MemberLevel: "vip"})
	goldConfig, _ := json.Marshal(models.MemberDiscountConfig{MemberLevel: "gold"})

	return []models.PricingRule{
		{
			Name: "早场优惠", RuleType: models.RuleTypeTimeSlot, Operator: models.OperatorMultiply,
			Value: 0.8, Description: "早场6-12点享受8折优惠", IsActive: true,
			Config: string(morningConfig),
		},
		{
			Name: "午场标准价", RuleType: models.RuleTypeTimeSlot, Operator: models.OperatorSet,
			Value: 160, Description: "午场12-18点标准价格", IsActive: true,
			Config: string(afternoonConfig),
		},
		{
			Name: "晚场高峰加价", RuleType: models.RuleTypeTimeSlot, Operator: models.OperatorMultiply,
			Value: 1.5, Description: "晚场18-22点高峰时段加价50%", IsActive: true,
			Config: string(eveningConfig),
		},
		{
			Name: "工作日折扣", RuleType: models.RuleTypeWeekday, Operator: models.OperatorMultiply,
			Value: 0.9, Description: "工作日享受9折", IsActive: true,
			Config: string(weekdayConfig),
		},
		{
			Name: "周末加价", RuleType: models.RuleTypeWeekday, Operator: models.OperatorMultiply,
			Value: 1.2, Description: "周末价格上浮20%", IsActive: true,
			Config: string(weekendConfig),
		},
		{
			Name: "节假日加价", RuleType: models.RuleTypeHoliday, Operator: models.OperatorMultiply,
			Value: 1.3, Description: "法定节假日价格上浮30%", IsActive: true,
			Config: string(holidayConfig),
		},
		{
			Name: "提前7天预订优惠", RuleType: models.RuleTypeAdvanceBooking, Operator: models.OperatorMultiply,
			Value: 0.85, Description: "提前7天以上预订享受85折", IsActive: true,
			Config: string(advance7Config),
		},
		{
			Name: "提前3天预订优惠", RuleType: models.RuleTypeAdvanceBooking, Operator: models.OperatorMultiply,
			Value: 0.95, Description: "提前3-6天预订享受95折", IsActive: true,
			Config: string(advance3Config),
		},
		{
			Name: "临近预订加价", RuleType: models.RuleTypeAdvanceBooking, Operator: models.OperatorMultiply,
			Value: 1.1, Description: "提前0-2天预订加价10%", IsActive: true,
			Config: string(advance1Config),
		},
		{
			Name: "低上座率促销", RuleType: models.RuleTypeOccupancy, Operator: models.OperatorMultiply,
			Value: 0.7, Description: "上座率低于30%时7折促销", IsActive: true,
			Config: string(lowOccConfig),
		},
		{
			Name: "中上座率标准价", RuleType: models.RuleTypeOccupancy, Operator: models.OperatorMultiply,
			Value: 1.0, Description: "上座率30%-70%标准价格", IsActive: true,
			Config: string(midOccConfig),
		},
		{
			Name: "高上座率加价", RuleType: models.RuleTypeOccupancy, Operator: models.OperatorMultiply,
			Value: 1.3, Description: "上座率超过70%时加价30%", IsActive: true,
			Config: string(highOccConfig),
		},
		{
			Name: "VIP会员折扣", RuleType: models.RuleTypeMemberDiscount, Operator: models.OperatorMultiply,
			Value: 0.8, Description: "VIP会员享受8折优惠", IsActive: true,
			Config: string(vipConfig),
		},
		{
			Name: "黄金会员折扣", RuleType: models.RuleTypeMemberDiscount, Operator: models.OperatorMultiply,
			Value: 0.9, Description: "黄金会员享受9折优惠", IsActive: true,
			Config: string(goldConfig),
		},
	}
}

func createSeedBindings(rules []models.PricingRule, venues []models.Venue) []models.VenuePricingBinding {
	ruleMap := make(map[string]*uint)
	for _, r := range rules {
		id := r.ID
		ruleMap[r.Name] = &id
	}

	var bindings []models.VenuePricingBinding

	for _, venue := range venues {
		if venue.SportType == "basketball" {
			bindings = append(bindings,
				models.VenuePricingBinding{
					VenueID: venue.ID, RuleID: ruleMap["晚场高峰加价"], Priority: 100, IsActive: true,
				},
				models.VenuePricingBinding{
					VenueID: venue.ID, RuleID: ruleMap["早场优惠"], Priority: 90, IsActive: true,
				},
				models.VenuePricingBinding{
					VenueID: venue.ID, RuleID: ruleMap["周末加价"], Priority: 80, IsActive: true,
				},
				models.VenuePricingBinding{
					VenueID: venue.ID, RuleID: ruleMap["工作日折扣"], Priority: 80, IsActive: true,
				},
				models.VenuePricingBinding{
					VenueID: venue.ID, RuleID: ruleMap["节假日加价"], Priority: 95, IsActive: true,
				},
				models.VenuePricingBinding{
					VenueID: venue.ID, RuleID: ruleMap["提前7天预订优惠"], Priority: 70, IsActive: true,
				},
				models.VenuePricingBinding{
					VenueID: venue.ID, RuleID: ruleMap["临近预订加价"], Priority: 75, IsActive: true,
				},
				models.VenuePricingBinding{
					VenueID: venue.ID, RuleID: ruleMap["高上座率加价"], Priority: 85, IsActive: true,
				},
				models.VenuePricingBinding{
					VenueID: venue.ID, RuleID: ruleMap["低上座率促销"], Priority: 85, IsActive: true,
				},
				models.VenuePricingBinding{
					VenueID: venue.ID, RuleID: ruleMap["VIP会员折扣"], Priority: 110, IsActive: true,
				},
				models.VenuePricingBinding{
					VenueID: venue.ID, RuleID: ruleMap["黄金会员折扣"], Priority: 105, IsActive: true,
				},
				models.VenuePricingBinding{
					VenueID: venue.ID, RuleID: nil, Priority: 0, IsActive: true, MinPrice: 80, MaxPrice: 400,
				},
			)
		} else if venue.SportType == "swimming" {
			bindings = append(bindings,
				models.VenuePricingBinding{
					VenueID: venue.ID, RuleID: ruleMap["早场优惠"], Priority: 90, IsActive: true,
				},
				models.VenuePricingBinding{
					VenueID: venue.ID, RuleID: ruleMap["晚场高峰加价"], Priority: 100, IsActive: true,
				},
				models.VenuePricingBinding{
					VenueID: venue.ID, RuleID: ruleMap["周末加价"], Priority: 80, IsActive: true,
				},
				models.VenuePricingBinding{
					VenueID: venue.ID, RuleID: ruleMap["节假日加价"], Priority: 95, IsActive: true,
				},
				models.VenuePricingBinding{
					VenueID: venue.ID, RuleID: nil, Priority: 0, IsActive: true, MinPrice: 40, MaxPrice: 200,
				},
			)
		} else if venue.SportType == "football" {
			bindings = append(bindings,
				models.VenuePricingBinding{
					VenueID: venue.ID, RuleID: ruleMap["周末加价"], Priority: 80, IsActive: true,
				},
				models.VenuePricingBinding{
					VenueID: venue.ID, RuleID: ruleMap["节假日加价"], Priority: 95, IsActive: true,
				},
				models.VenuePricingBinding{
					VenueID: venue.ID, RuleID: ruleMap["工作日折扣"], Priority: 80, IsActive: true,
				},
				models.VenuePricingBinding{
					VenueID: venue.ID, RuleID: nil, Priority: 0, IsActive: true, MinPrice: 150, MaxPrice: 600,
				},
			)
		}
	}

	return bindings
}

func createSeedHolidays() []models.Holiday {
	return []models.Holiday{
		{Date: "2026-01-01", Name: "元旦", IsWorkday: false},
		{Date: "2026-02-16", Name: "春节", IsWorkday: false},
		{Date: "2026-02-17", Name: "春节", IsWorkday: false},
		{Date: "2026-02-18", Name: "春节", IsWorkday: false},
		{Date: "2026-02-19", Name: "春节", IsWorkday: false},
		{Date: "2026-02-20", Name: "春节", IsWorkday: false},
		{Date: "2026-02-14", Name: "春节调休", IsWorkday: true},
		{Date: "2026-02-22", Name: "春节调休", IsWorkday: true},
		{Date: "2026-04-04", Name: "清明节", IsWorkday: false},
		{Date: "2026-04-05", Name: "清明节", IsWorkday: false},
		{Date: "2026-04-06", Name: "清明节", IsWorkday: false},
		{Date: "2026-05-01", Name: "劳动节", IsWorkday: false},
		{Date: "2026-05-02", Name: "劳动节", IsWorkday: false},
		{Date: "2026-05-03", Name: "劳动节", IsWorkday: false},
		{Date: "2026-05-04", Name: "劳动节", IsWorkday: false},
		{Date: "2026-05-05", Name: "劳动节", IsWorkday: false},
		{Date: "2026-04-26", Name: "劳动节调休", IsWorkday: true},
		{Date: "2026-05-09", Name: "劳动节调休", IsWorkday: true},
		{Date: "2026-06-19", Name: "端午节", IsWorkday: false},
		{Date: "2026-06-20", Name: "端午节", IsWorkday: false},
		{Date: "2026-06-21", Name: "端午节", IsWorkday: false},
		{Date: "2026-10-01", Name: "国庆节", IsWorkday: false},
		{Date: "2026-10-02", Name: "国庆节", IsWorkday: false},
		{Date: "2026-10-03", Name: "国庆节", IsWorkday: false},
		{Date: "2026-10-04", Name: "国庆节", IsWorkday: false},
		{Date: "2026-10-05", Name: "国庆节", IsWorkday: false},
		{Date: "2026-10-06", Name: "国庆节", IsWorkday: false},
		{Date: "2026-10-07", Name: "国庆节", IsWorkday: false},
		{Date: "2026-09-27", Name: "国庆调休", IsWorkday: true},
		{Date: "2026-10-10", Name: "国庆调休", IsWorkday: true},
	}
}

func createSeedPromotions() []models.Promotion {
	now := time.Now()
	startTime := now
	endTime := now.AddDate(0, 1, 0)

	weekdayJSON, _ := json.Marshal([]int{1, 2, 3, 4})

	return []models.Promotion{
		{
			Name: "开业限时8折", PromotionType: models.PromotionTypeDiscount,
			Description: "限时全场8折优惠活动",
			StartTime:   startTime, EndTime: endTime,
			DiscountRate: 0.8, IsActive: true,
		},
		{
			Name: "工作日满减活动", PromotionType: models.PromotionTypeFullReduce,
			Description: "工作日满200减30",
			StartTime:   startTime, EndTime: endTime,
			Weekdays: string(weekdayJSON),
			FullAmount: 200, ReduceAmount: 30, IsActive: true,
		},
	}
}

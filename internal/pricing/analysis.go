package pricing

import (
	"fmt"
	"strconv"

	"gorm.io/gorm"

	"venue-booking-admin/internal/models"
)

// AnalysisService 定价分析服务
type AnalysisService struct {
	db *gorm.DB
}

// NewAnalysisService 创建分析服务
func NewAnalysisService(db *gorm.DB) *AnalysisService {
	return &AnalysisService{db: db}
}

// PriceDistribution 价格分布
type PriceDistribution struct {
	PriceRange string  `json:"price_range"`
	Count      int64   `json:"count"`
	Percentage float64 `json:"percentage"`
}

// RevenueAnalysis 营收分析
type RevenueAnalysis struct {
	TotalRevenue      float64 `json:"total_revenue"`
	FixedPriceRevenue float64 `json:"fixed_price_revenue"`
	DynamicPriceGain  float64 `json:"dynamic_price_gain"`
	GainPercentage    float64 `json:"gain_percentage"`
	TotalBookings     int64   `json:"total_bookings"`
	AvgPrice          float64 `json:"avg_price"`
	AvgFixedPrice     float64 `json:"avg_fixed_price"`
}

// BookingPriceStats 预订价格统计
type BookingPriceStats struct {
	VenueID       uint    `json:"venue_id"`
	VenueName     string  `json:"venue_name"`
	MinPrice      float64 `json:"min_price"`
	MaxPrice      float64 `json:"max_price"`
	AvgPrice      float64 `json:"avg_price"`
	TotalBookings int64   `json:"total_bookings"`
	TotalRevenue  float64 `json:"total_revenue"`
}

// SimulateResult 模拟结果
type SimulateResult struct {
	VenueID        uint        `json:"venue_id"`
	VenueName      string      `json:"venue_name"`
	Date           string      `json:"date"`
	Occupancy      float64     `json:"occupancy"`
	FixedRevenue   float64     `json:"fixed_revenue"`
	DynamicRevenue float64     `json:"dynamic_revenue"`
	GainAmount     float64     `json:"gain_amount"`
	GainPercentage float64     `json:"gain_percentage"`
	HourPrices     []HourPrice `json:"hour_prices"`
}

// GetPriceDistribution 获取价格分布统计
func (s *AnalysisService) GetPriceDistribution(venueID uint, startDate, endDate string, ranges []float64) ([]PriceDistribution, error) {
	if len(ranges) < 2 {
		ranges = []float64{0, 50, 100, 150, 200, 300, 500, 1000}
	}

	var totalBookings int64
	query := s.db.Model(&models.Booking{}).Where("status <> ?", "cancelled")
	if venueID > 0 {
		query = query.Where("venue_id = ?", venueID)
	}
	if startDate != "" {
		query = query.Where("book_date >= ?", startDate)
	}
	if endDate != "" {
		query = query.Where("book_date <= ?", endDate)
	}
	query.Count(&totalBookings)

	if totalBookings == 0 {
		return []PriceDistribution{}, nil
	}

	var result []PriceDistribution
	for i := 0; i < len(ranges)-1; i++ {
		var count int64
		q := s.db.Model(&models.Booking{}).
			Where("status <> ?", "cancelled").
			Where("amount / (end_hour - start_hour) >= ? AND amount / (end_hour - start_hour) < ?", ranges[i], ranges[i+1])

		if venueID > 0 {
			q = q.Where("venue_id = ?", venueID)
		}
		if startDate != "" {
			q = q.Where("book_date >= ?", startDate)
		}
		if endDate != "" {
			q = q.Where("book_date <= ?", endDate)
		}

		q.Count(&count)

		priceRange := formatPriceRange(ranges[i], ranges[i+1])
		percentage := float64(count) / float64(totalBookings) * 100

		result = append(result, PriceDistribution{
			PriceRange: priceRange,
			Count:      count,
			Percentage: roundPrice(percentage),
		})
	}

	return result, nil
}

// GetRevenueAnalysis 获取营收分析
func (s *AnalysisService) GetRevenueAnalysis(venueID uint, startDate, endDate string) (*RevenueAnalysis, error) {
	var bookings []models.Booking
	query := s.db.Where("status <> ?", "cancelled")
	if venueID > 0 {
		query = query.Where("venue_id = ?", venueID)
	}
	if startDate != "" {
		query = query.Where("book_date >= ?", startDate)
	}
	if endDate != "" {
		query = query.Where("book_date <= ?", endDate)
	}
	query.Find(&bookings)

	if len(bookings) == 0 {
		return &RevenueAnalysis{}, nil
	}

	var totalRevenue, fixedPriceRevenue float64
	var totalHours float64

	venueMap := make(map[uint]float64)

	for _, booking := range bookings {
		totalRevenue += booking.Amount
		hours := float64(booking.EndHour - booking.StartHour)
		totalHours += hours

		basePrice, ok := venueMap[booking.VenueID]
		if !ok {
			var venue models.Venue
			if err := s.db.First(&venue, booking.VenueID).Error; err == nil {
				basePrice = venue.HourlyPrice
				venueMap[booking.VenueID] = basePrice
			}
		}
		fixedPriceRevenue += basePrice * hours
	}

	dynamicGain := totalRevenue - fixedPriceRevenue
	gainPercentage := 0.0
	if fixedPriceRevenue > 0 {
		gainPercentage = dynamicGain / fixedPriceRevenue * 100
	}

	avgPrice := 0.0
	avgFixedPrice := 0.0
	if totalHours > 0 {
		avgPrice = totalRevenue / totalHours
		avgFixedPrice = fixedPriceRevenue / totalHours
	}

	return &RevenueAnalysis{
		TotalRevenue:      roundPrice(totalRevenue),
		FixedPriceRevenue: roundPrice(fixedPriceRevenue),
		DynamicPriceGain:  roundPrice(dynamicGain),
		GainPercentage:    roundPrice(gainPercentage),
		TotalBookings:     int64(len(bookings)),
		AvgPrice:          roundPrice(avgPrice),
		AvgFixedPrice:     roundPrice(avgFixedPrice),
	}, nil
}

// GetVenuePriceStats 获取各场馆价格统计
func (s *AnalysisService) GetVenuePriceStats(startDate, endDate string) ([]BookingPriceStats, error) {
	var venues []models.Venue
	s.db.Find(&venues)

	var result []BookingPriceStats

	for _, venue := range venues {
		var stats BookingPriceStats
		stats.VenueID = venue.ID
		stats.VenueName = venue.Name

		var bookings []models.Booking
		query := s.db.Where("venue_id = ? AND status <> ?", venue.ID, "cancelled")
		if startDate != "" {
			query = query.Where("book_date >= ?", startDate)
		}
		if endDate != "" {
			query = query.Where("book_date <= ?", endDate)
		}
		query.Find(&bookings)

		if len(bookings) == 0 {
			continue
		}

		var totalRevenue float64
		var totalHours float64
		minPrice := 0.0
		maxPrice := 0.0
		first := true

		for _, booking := range bookings {
			totalRevenue += booking.Amount
			hours := float64(booking.EndHour - booking.StartHour)
			totalHours += hours

			hourlyPrice := booking.Amount / hours
			if first || hourlyPrice < minPrice {
				minPrice = hourlyPrice
			}
			if first || hourlyPrice > maxPrice {
				maxPrice = hourlyPrice
			}
			first = false
		}

		stats.TotalBookings = int64(len(bookings))
		stats.TotalRevenue = roundPrice(totalRevenue)
		stats.MinPrice = roundPrice(minPrice)
		stats.MaxPrice = roundPrice(maxPrice)
		if totalHours > 0 {
			stats.AvgPrice = roundPrice(totalRevenue / totalHours)
		}

		result = append(result, stats)
	}

	return result, nil
}

// SimulateRevenueImpact 模拟调价对营收的影响
func (s *AnalysisService) SimulateRevenueImpact(venueID uint, date string, occupancy float64, memberLevel string) (*SimulateResult, error) {
	var venue models.Venue
	if err := s.db.First(&venue, venueID).Error; err != nil {
		return nil, err
	}

	engine := NewEngine(s.db)
	hourPrices, err := engine.SimulateDayPrice(venueID, date, occupancy, memberLevel)
	if err != nil {
		return nil, err
	}

	var dynamicRevenue, fixedRevenue float64

	for _, hp := range hourPrices {
		dynamicRevenue += hp.Price
		fixedRevenue += venue.HourlyPrice
	}

	gainAmount := dynamicRevenue - fixedRevenue
	gainPercentage := 0.0
	if fixedRevenue > 0 {
		gainPercentage = gainAmount / fixedRevenue * 100
	}

	return &SimulateResult{
		VenueID:        venueID,
		VenueName:      venue.Name,
		Date:           date,
		Occupancy:      occupancy,
		FixedRevenue:   roundPrice(fixedRevenue),
		DynamicRevenue: roundPrice(dynamicRevenue),
		GainAmount:     roundPrice(gainAmount),
		GainPercentage: roundPrice(gainPercentage),
		HourPrices:     hourPrices,
	}, nil
}

func formatPriceRange(min, max float64) string {
	return fmt.Sprintf("%s-%s", formatFloat(min), formatFloat(max))
}

func formatFloat(f float64) string {
	return strconv.FormatFloat(f, 'f', 0, 64)
}

package main

import (
	"log"

	"github.com/gin-gonic/gin"

	"venue-booking-admin/internal/auth"
	"venue-booking-admin/internal/config"
	"venue-booking-admin/internal/db"
	"venue-booking-admin/internal/handlers"
	"venue-booking-admin/internal/pricing"
	"venue-booking-admin/internal/seed"
)

func main() {
	cfg := config.Load()
	auth.SetSecret(cfg.JWTSecret)

	database, err := db.Connect(cfg.DSN)
	if err != nil {
		log.Fatalf("无法连接数据库: %v", err)
	}
	if err := db.Migrate(database); err != nil {
		log.Fatalf("数据库迁移失败: %v", err)
	}
	if err := seed.Run(database, cfg.AdminUsername, cfg.AdminPassword); err != nil {
		log.Fatalf("种子数据初始化失败: %v", err)
	}

	pricingEngine := pricing.NewEngine(database)
	lockService := pricing.NewLockService(database, pricingEngine)

	h := &handlers.Handler{
		DB:      database,
		Pricing: pricingEngine,
		LockSvc: lockService,
	}
	ph := handlers.NewPricingHandler(database)

	gin.SetMode(gin.ReleaseMode)
	r := gin.Default()

	api := r.Group("/api")
	{
		api.GET("/health", h.Health)
		api.POST("/auth/login", h.Login)

		secured := api.Group("")
		secured.Use(auth.Middleware(database))
		{
			secured.GET("/auth/me", h.Me)

			secured.GET("/venues", h.ListVenues)
			secured.POST("/venues", h.CreateVenue)
			secured.GET("/venues/:id", h.GetVenue)
			secured.PUT("/venues/:id", h.UpdateVenue)
			secured.DELETE("/venues/:id", h.DeleteVenue)
			secured.GET("/venues/:venue_id/occupancy", ph.GetOccupancy)

			secured.GET("/bookings", h.ListBookings)
			secured.POST("/bookings", h.CreateBooking)
			secured.PATCH("/bookings/:id/status", h.UpdateBookingStatus)

			secured.GET("/dashboard/stats", h.DashboardStats)

			// 定价规则管理
			secured.GET("/pricing/rules", ph.ListPricingRules)
			secured.POST("/pricing/rules", ph.CreatePricingRule)
			secured.GET("/pricing/rules/:id", ph.GetPricingRule)
			secured.PUT("/pricing/rules/:id", ph.UpdatePricingRule)
			secured.DELETE("/pricing/rules/:id", ph.DeletePricingRule)

			// 规则绑定管理
			secured.GET("/pricing/bindings", ph.ListVenueBindings)
			secured.POST("/pricing/bindings", ph.CreateBinding)
			secured.PUT("/pricing/bindings/:id", ph.UpdateBinding)
			secured.DELETE("/pricing/bindings/:id", ph.DeleteBinding)

			// 实时查价
			secured.POST("/pricing/calculate", ph.CalculatePrice)

			// 价格锁
			secured.POST("/pricing/lock", ph.LockPrice)
			secured.GET("/pricing/lock/:lock_key", ph.GetLockedPrice)

			// 节假日管理
			secured.GET("/pricing/holidays", ph.ListHolidays)
			secured.POST("/pricing/holidays", ph.CreateHoliday)
			secured.PUT("/pricing/holidays/:id", ph.UpdateHoliday)
			secured.DELETE("/pricing/holidays/:id", ph.DeleteHoliday)

			// 价格审计日志
			secured.GET("/pricing/audit-logs", ph.ListAuditLogs)

			// 调价模拟
			secured.POST("/pricing/simulate", ph.SimulateDayPrice)

			// 促销活动管理
			secured.GET("/pricing/promotions", ph.ListPromotions)
			secured.POST("/pricing/promotions", ph.CreatePromotion)
			secured.GET("/pricing/promotions/:id", ph.GetPromotion)
			secured.PUT("/pricing/promotions/:id", ph.UpdatePromotion)
			secured.DELETE("/pricing/promotions/:id", ph.DeletePromotion)

			// 灰度发布管理
			secured.GET("/pricing/gray-releases", ph.ListGrayReleases)
			secured.POST("/pricing/gray-releases", ph.CreateGrayRelease)
			secured.PUT("/pricing/gray-releases/:id", ph.UpdateGrayRelease)
			secured.DELETE("/pricing/gray-releases/:id", ph.DeleteGrayRelease)

			// 定价效果分析
			secured.GET("/pricing/analysis/distribution", ph.GetPriceDistribution)
			secured.GET("/pricing/analysis/revenue", ph.GetRevenueAnalysis)
			secured.GET("/pricing/analysis/venue-stats", ph.GetVenuePriceStats)
		}
	}

	log.Printf("venue-booking-admin listening on :%s", cfg.Port)
	if err := r.Run("0.0.0.0:" + cfg.Port); err != nil {
		log.Fatal(err)
	}
}

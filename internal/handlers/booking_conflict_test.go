package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	_ "modernc.org/sqlite"

	"venue-booking-admin/internal/models"
	"venue-booking-admin/internal/pricing"
)

// newBookingTestServer 起一个只挂载预订相关路由的测试服务，
// 使用内存 SQLite，并预置一个开放时间 8:00-22:00、基础价 50 的场馆。
func newBookingTestServer(t *testing.T) (*gin.Engine, models.Venue) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	db, err := gorm.Open(sqlite.Dialector{DriverName: "sqlite", DSN: ":memory:"}, &gorm.Config{})
	assert.NoError(t, err)
	assert.NoError(t, db.AutoMigrate(
		&models.Venue{},
		&models.Booking{},
		&models.PricingRule{},
		&models.VenuePricingBinding{},
		&models.Holiday{},
		&models.Promotion{},
		&models.PriceLock{},
		&models.PriceAuditLog{},
		&models.GrayRelease{},
	))

	venue := models.Venue{
		Name: "市民广场羽毛球馆", SportType: "badminton",
		Capacity: 60, HourlyPrice: 50,
		OpenHour: 8, CloseHour: 22, Status: "open",
	}
	assert.NoError(t, db.Create(&venue).Error)

	engine := pricing.NewEngine(db)
	h := &Handler{DB: db, Pricing: engine, LockSvc: pricing.NewLockService(db, engine)}

	router := gin.New()
	router.POST("/api/bookings", h.CreateBooking)
	router.GET("/api/bookings", h.ListBookings)
	router.PATCH("/api/bookings/:id/status", h.UpdateBookingStatus)
	return router, venue
}

func postBooking(t *testing.T, router *gin.Engine, venueID uint, customer, date string, startHour, endHour int) *httptest.ResponseRecorder {
	t.Helper()

	body, err := json.Marshal(map[string]interface{}{
		"venue_id":      venueID,
		"customer_name": customer,
		"phone":         "13700000000",
		"book_date":     date,
		"start_hour":    startHour,
		"end_hour":      endHour,
	})
	assert.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/bookings", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	return recorder
}

func patchBookingStatus(t *testing.T, router *gin.Engine, bookingID uint, status string) *httptest.ResponseRecorder {
	t.Helper()

	body, err := json.Marshal(map[string]string{"status": status})
	assert.NoError(t, err)

	req := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/bookings/%d/status", bookingID), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	return recorder
}

func decodeBooking(t *testing.T, recorder *httptest.ResponseRecorder) models.Booking {
	t.Helper()

	var booking models.Booking
	assert.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &booking))
	return booking
}

// TestCreateBookingAcceptsBackToBackSlots 断言首尾相接但互不重叠的时段可以连续预订：
// 已有 10:00-12:00 的预订时，12:00-14:00 与 8:00-10:00 都应登记成功。
func TestCreateBookingAcceptsBackToBackSlots(t *testing.T) {
	router, venue := newBookingTestServer(t)

	first := postBooking(t, router, venue.ID, "陈刚", "2026-06-23", 10, 12)
	assert.Equal(t, http.StatusCreated, first.Code, first.Body.String())
	assert.Equal(t, 100.0, decodeBooking(t, first).Amount)

	after := postBooking(t, router, venue.ID, "周敏", "2026-06-23", 12, 14)
	assert.Equal(t, http.StatusCreated, after.Code, after.Body.String())

	before := postBooking(t, router, venue.ID, "黄磊", "2026-06-23", 8, 10)
	assert.Equal(t, http.StatusCreated, before.Code, before.Body.String())

	third := postBooking(t, router, venue.ID, "吴静", "2026-06-23", 14, 16)
	assert.Equal(t, http.StatusCreated, third.Code, third.Body.String())

	listReq := httptest.NewRequest(http.MethodGet, "/api/bookings?venue_id="+fmt.Sprint(venue.ID)+"&date=2026-06-23", nil)
	listRecorder := httptest.NewRecorder()
	router.ServeHTTP(listRecorder, listReq)
	assert.Equal(t, http.StatusOK, listRecorder.Code)

	var bookings []models.Booking
	assert.NoError(t, json.Unmarshal(listRecorder.Body.Bytes(), &bookings))
	assert.Len(t, bookings, 4)

	slots := map[string]bool{}
	for _, b := range bookings {
		slots[fmt.Sprintf("%d-%d", b.StartHour, b.EndHour)] = true
	}
	assert.Equal(t, map[string]bool{"8-10": true, "10-12": true, "12-14": true, "14-16": true}, slots)
}

// TestCreateBookingRejectsOverlappingSlots 断言真正重叠的时段仍然会被拒绝并返回 409。
func TestCreateBookingRejectsOverlappingSlots(t *testing.T) {
	router, venue := newBookingTestServer(t)

	first := postBooking(t, router, venue.ID, "陈刚", "2026-06-23", 10, 12)
	assert.Equal(t, http.StatusCreated, first.Code, first.Body.String())

	overlaps := [][2]int{{10, 12}, {9, 11}, {11, 13}, {9, 13}, {11, 12}}
	for _, slot := range overlaps {
		recorder := postBooking(t, router, venue.ID, "冲突客户", "2026-06-23", slot[0], slot[1])
		assert.Equal(t, http.StatusConflict, recorder.Code, fmt.Sprintf("slot %d-%d 应判定为冲突", slot[0], slot[1]))
	}
}

// TestCreateBookingIgnoresCancelledAndOtherDays 断言已取消的预订与其他日期的预订不占用时段。
func TestCreateBookingIgnoresCancelledAndOtherDays(t *testing.T) {
	router, venue := newBookingTestServer(t)

	first := postBooking(t, router, venue.ID, "陈刚", "2026-06-23", 10, 12)
	assert.Equal(t, http.StatusCreated, first.Code, first.Body.String())

	sameSlotOtherDay := postBooking(t, router, venue.ID, "周敏", "2026-06-24", 10, 12)
	assert.Equal(t, http.StatusCreated, sameSlotOtherDay.Code, sameSlotOtherDay.Body.String())

	cancelled := patchBookingStatus(t, router, decodeBooking(t, first).ID, "cancelled")
	assert.Equal(t, http.StatusOK, cancelled.Code, cancelled.Body.String())

	rebooked := postBooking(t, router, venue.ID, "黄磊", "2026-06-23", 10, 12)
	assert.Equal(t, http.StatusCreated, rebooked.Code, rebooked.Body.String())
}

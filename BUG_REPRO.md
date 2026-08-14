# Bug Reproduction

## 包的性质

当前 test_model_fix 保存的是被测模型修复后的结果源码，不是初始含 Bug 源码。要复现原始缺陷，必须检出下面固定的 parent SHA，再应用仓库内的可信测试补丁；不要在当前修复结果源码上期待重新出现修复前失败。

## 问题现象

预订登记有个时段冲突误判，帮我修一下。

场馆前台反馈：同一个场馆同一天，已经有一笔 10:00-12:00 的预订之后，再登记 12:00-14:00 会被拒掉，POST /api/bookings 返回 409 {"detail":"该时段已被预订"}。但 12:00 这一笔跟前面那笔是首尾相接、并不重叠，按规则应该可以订。

更奇怪的是它有方向性：
- 已有 10:00-12:00，再订 12:00-14:00 → 被拒（不应该拒）
- 已有 10:00-12:00，再订 8:00-10:00 → 正常登记成功
也就是说只有「紧接在已有预订之后开始」的时段会被误判，紧接在前面的没事。结果连排时段没法一笔一笔录进去，前台只能把后一笔的开始时间往后挪一小时，白丢一小时收入。

时段用的是半开区间 [start_hour, end_hour)。期望行为：
- 只有真正重叠的时段才返回 409：已有 10-12 时，10-12、9-11、11-13、9-13、11-12 都算冲突；
- 首尾相接的时段必须能登记成功：已有 10-12 时，12-14 和 8-10 都要成功，连排 8-10 / 10-12 / 12-14 / 14-16 四笔应该全部录得进去；
- 状态为 cancelled 的预订不占时段，其他日期的同时段预订也不互相影响；
- 金额仍按现有定价逻辑计算，不要动算费。

修完请保证 go test ./... 全绿。

## 含 Bug 版本

- 仓库：zhanglei10281852-gif/gogo-04
- 仓库地址：https://github.com/zhanglei10281852-gif/gogo-04.git
- parent SHA：75d650011188768e836071f706536455521ba1e2

## 复现步骤

```bash
git clone -- https://github.com/zhanglei10281852-gif/gogo-04.git bug-repro
cd bug-repro
git checkout --detach 75d650011188768e836071f706536455521ba1e2
git apply ../BENZHI_VALIDATION/trusted-test.patch
go test -count=1 -v -run "TestCreateBookingAcceptsBackToBackSlots|TestCreateBookingRejectsOverlappingSlots|TestCreateBookingIgnoresCancelledAndOtherDays" ./internal/handlers/
```

## 双架构完整错误信息

### linux/amd64

- 容器内复现预期退出码：1
- 容器内复现实际退出码：1

stdout：

```text
$ go test -count=1 -v -run "TestCreateBookingAcceptsBackToBackSlots|TestCreateBookingRejectsOverlappingSlots|TestCreateBookingIgnoresCancelledAndOtherDays" ./internal/handlers/
=== RUN   TestCreateBookingAcceptsBackToBackSlots

2026/08/14 21:28:24 /app/internal/pricing/engine.go:180 record not found
[0.016ms] [rows:0] SELECT * FROM `holidays` WHERE date = "2026-06-23" ORDER BY `holidays`.`id` LIMIT 1
    booking_conflict_test.go:109: 
        	Error Trace:	/app/internal/handlers/booking_conflict_test.go:109
        	Error:      	Not equal: 
        	            	expected: 201
        	            	actual  : 409
        	Test:       	TestCreateBookingAcceptsBackToBackSlots
        	Messages:   	{"detail":"该时段已被预订"}

2026/08/14 21:28:24 /app/internal/pricing/engine.go:180 record not found
[0.016ms] [rows:0] SELECT * FROM `holidays` WHERE date = "2026-06-23" ORDER BY `holidays`.`id` LIMIT 1

2026/08/14 21:28:24 /app/internal/pricing/engine.go:180 record not found
[0.020ms] [rows:0] SELECT * FROM `holidays` WHERE date = "2026-06-23" ORDER BY `holidays`.`id` LIMIT 1
    booking_conflict_test.go:124: 
        	Error Trace:	/app/internal/handlers/booking_conflict_test.go:124
        	Error:      	"[{3 1 吴静 13700000000 2026-06-23 14 16 100 booked 2026-08-14 21:28:24.669213826 +0000 UTC} {2 1 黄磊 13700000000 2026-06-23 8 10 100 booked 2026-08-14 21:28:24.668656791 +0000 UTC} {1 1 陈刚 13700000000 2026-06-23 10 12 100 booked 2026-08-14 21:28:24.667782273 +0000 UTC}]" should have 4 item(s), but has 3
        	Test:       	TestCreateBookingAcceptsBackToBackSlots
    booking_conflict_test.go:130: 
        	Error Trace:	/app/internal/handlers/booking_conflict_test.go:130
        	Error:      	Not equal: 
        	            	expected: map[string]bool{"10-12":true, "12-14":true, "14-16":true, "8-10":true}
        	            	actual  : map[string]bool{"10-12":true, "14-16":true, "8-10":true}
        	            	
        	            	Diff:
        	            	--- Expected
        	            	+++ Actual
        	            	@@ -1,4 +1,3 @@
        	            	-(map[string]bool) (len=4) {
        	            	+(map[string]bool) (len=3) {
        	            	  (string) (len=5) "10-12": (bool) true,
        	            	- (string) (len=5) "12-14": (bool) true,
        	            	  (string) (len=5) "14-16": (bool) true,
        	Test:       	TestCreateBookingAcceptsBackToBackSlots
--- FAIL: TestCreateBookingAcceptsBackToBackSlots (0.01s)
=== RUN   TestCreateBookingRejectsOverlappingSlots

2026/08/14 21:28:24 /app/internal/pricing/engine.go:180 record not found
[0.021ms] [rows:0] SELECT * FROM `holidays` WHERE date = "2026-06-23" ORDER BY `holidays`.`id` LIMIT 1
--- PASS: TestCreateBookingRejectsOverlappingSlots (0.00s)
=== RUN   TestCreateBookingIgnoresCancelledAndOtherDays

2026/08/14 21:28:24 /app/internal/pricing/engine.go:180 record not found
[0.021ms] [rows:0] SELECT * FROM `holidays` WHERE date = "2026-06-23" ORDER BY `holidays`.`id` LIMIT 1

2026/08/14 21:28:24 /app/internal/pricing/engine.go:180 record not found
[0.031ms] [rows:0] SELECT * FROM `holidays` WHERE date = "2026-06-24" ORDER BY `holidays`.`id` LIMIT 1

2026/08/14 21:28:24 /app/internal/pricing/engine.go:180 record not found
[0.031ms] [rows:0] SELECT * FROM `holidays` WHERE date = "2026-06-23" ORDER BY `holidays`.`id` LIMIT 1
--- PASS: TestCreateBookingIgnoresCancelledAndOtherDays (0.01s)
FAIL
FAIL	venue-booking-admin/internal/handlers	0.029s
FAIL

```

stderr：

```text
warning: internal/handlers/booking_conflict_test.go has type 100755, expected 100644
warning: internal/handlers/booking_conflict_test.go has type 100755, expected 100644

```

### linux/arm64

- 容器内复现预期退出码：1
- 容器内复现实际退出码：1

stdout：

```text
$ go test -count=1 -v -run "TestCreateBookingAcceptsBackToBackSlots|TestCreateBookingRejectsOverlappingSlots|TestCreateBookingIgnoresCancelledAndOtherDays" ./internal/handlers/
=== RUN   TestCreateBookingAcceptsBackToBackSlots

2026/08/14 21:45:46 /app/internal/pricing/engine.go:180 record not found
[0.795ms] [rows:0] SELECT * FROM `holidays` WHERE date = "2026-06-23" ORDER BY `holidays`.`id` LIMIT 1
    booking_conflict_test.go:109: 
        	Error Trace:	/app/internal/handlers/booking_conflict_test.go:109
        	Error:      	Not equal: 
        	            	expected: 201
        	            	actual  : 409
        	Test:       	TestCreateBookingAcceptsBackToBackSlots
        	Messages:   	{"detail":"该时段已被预订"}

2026/08/14 21:45:46 /app/internal/pricing/engine.go:180 record not found
[0.303ms] [rows:0] SELECT * FROM `holidays` WHERE date = "2026-06-23" ORDER BY `holidays`.`id` LIMIT 1

2026/08/14 21:45:46 /app/internal/pricing/engine.go:180 record not found
[0.290ms] [rows:0] SELECT * FROM `holidays` WHERE date = "2026-06-23" ORDER BY `holidays`.`id` LIMIT 1
    booking_conflict_test.go:124: 
        	Error Trace:	/app/internal/handlers/booking_conflict_test.go:124
        	Error:      	"[{3 1 吴静 13700000000 2026-06-23 14 16 100 booked 2026-08-14 21:45:46.879257241 +0000 UTC} {2 1 黄磊 13700000000 2026-06-23 8 10 100 booked 2026-08-14 21:45:46.872191061 +0000 UTC} {1 1 陈刚 13700000000 2026-06-23 10 12 100 booked 2026-08-14 21:45:46.847878382 +0000 UTC}]" should have 4 item(s), but has 3
        	Test:       	TestCreateBookingAcceptsBackToBackSlots
    booking_conflict_test.go:130: 
        	Error Trace:	/app/internal/handlers/booking_conflict_test.go:130
        	Error:      	Not equal: 
        	            	expected: map[string]bool{"10-12":true, "12-14":true, "14-16":true, "8-10":true}
        	            	actual  : map[string]bool{"10-12":true, "14-16":true, "8-10":true}
        	            	
        	            	Diff:
        	            	--- Expected
        	            	+++ Actual
        	            	@@ -1,4 +1,3 @@
        	            	-(map[string]bool) (len=4) {
        	            	+(map[string]bool) (len=3) {
        	            	  (string) (len=5) "10-12": (bool) true,
        	            	- (string) (len=5) "12-14": (bool) true,
        	            	  (string) (len=5) "14-16": (bool) true,
        	Test:       	TestCreateBookingAcceptsBackToBackSlots
--- FAIL: TestCreateBookingAcceptsBackToBackSlots (0.33s)
=== RUN   TestCreateBookingRejectsOverlappingSlots

2026/08/14 21:45:46 /app/internal/pricing/engine.go:180 record not found
[0.330ms] [rows:0] SELECT * FROM `holidays` WHERE date = "2026-06-23" ORDER BY `holidays`.`id` LIMIT 1
--- PASS: TestCreateBookingRejectsOverlappingSlots (0.05s)
=== RUN   TestCreateBookingIgnoresCancelledAndOtherDays

2026/08/14 21:45:46 /app/internal/pricing/engine.go:180 record not found
[0.323ms] [rows:0] SELECT * FROM `holidays` WHERE date = "2026-06-23" ORDER BY `holidays`.`id` LIMIT 1

2026/08/14 21:45:46 /app/internal/pricing/engine.go:180 record not found
[0.276ms] [rows:0] SELECT * FROM `holidays` WHERE date = "2026-06-24" ORDER BY `holidays`.`id` LIMIT 1

2026/08/14 21:45:47 /app/internal/pricing/engine.go:180 record not found
[0.354ms] [rows:0] SELECT * FROM `holidays` WHERE date = "2026-06-23" ORDER BY `holidays`.`id` LIMIT 1
--- PASS: TestCreateBookingIgnoresCancelledAndOtherDays (0.08s)
FAIL
FAIL	venue-booking-admin/internal/handlers	0.859s
FAIL

```

stderr：

```text
warning: internal/handlers/booking_conflict_test.go has type 100755, expected 100644
warning: internal/handlers/booking_conflict_test.go has type 100755, expected 100644

```

## 通过条件

通过标准：
1. 定向复现命令在修复后全部通过：go test ./internal/handlers/ -run 'TestCreateBookingAcceptsBackToBackSlots|TestCreateBookingRejectsOverlappingSlots|TestCreateBookingIgnoresCancelledAndOtherDays' -count=1 -v
2. 全量回归通过：go test ./... -count=1（本仓库全量可跑，未做范围收敛）
3. linux/amd64 与 linux/arm64 两个架构下 go build ./... 与 go test ./... 均通过
4. 断言的是公开 HTTP 行为：首尾相接时段返回 201 且能在列表接口查到 4 笔连排预订；真正重叠的 5 种时段返回 409；cancelled 与跨日期不占时段
5. 不得通过修改或跳过测试、放宽断言使结果变绿

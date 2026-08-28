# 為什麼並發打 20 個請求，`WaitCount` 卻是 0

**日期**：2026-08-28
**相關程式**：`internal/database/database.go`、`test-case/concurrentCreateOrder.go`、`main.go` 的 `/debug/pool` 與 `/debug/slow`

---

## 1. 當初的問題

我把連線池上限設成 3：

```go
db.SetMaxOpenConns(3)
db.SetMaxIdleConns(2)
db.SetConnMaxLifetime(50 * time.Second)
```

> ⚠️ 這 `3 / 2` 是**實驗當下臨時調小**的值，為了讓 20 個請求容易撞在一起。
> 實驗結束後已改回 `SetMaxOpenConns(25)` / `SetMaxIdleConns(5)`，
> 所以現在讀 `database.go` 會看到不一樣的數字 —— 本文所有數字都以 `3 / 2` 為前提。

然後寫測試用 20 個 goroutine 同時打 `GET /api/v1/orders`，預期會看到請求排隊搶連線。
結果每一份 `db.Stats()` 都長這樣：

```json
{"MaxOpenConnections":3,"OpenConnections":1,"InUse":0,"Idle":1,"WaitCount":0,...}
```

**我原本以為**：測試寫錯了、或者統計沒抓到瞬間。
**實際上**：測試沒錯，是查詢太快，「同時」這件事根本沒發生。

---

## 2. 結論

> **20 個並發 HTTP 請求 ≠ 20 個並發 DB 查詢。**

一個請求的生命週期裡，真正佔用 DB 連線的只是很小一片（單表 SELECT，次毫秒級）。
而請求之間的抵達時間差（goroutine 排程、TCP 建立、gin 路由、JWT 驗證）是毫秒級。

當「佔用連線的時間」比「請求之間的時間差」小一個數量級，連線被歸還的速度永遠快過
下一個請求要它的速度 —— **一條連線就足以接力服務完 20 個請求**。

要製造爭用，必須讓 DB 佔用時間遠大於抵達時間差。做法：`SELECT pg_sleep(1)`。

---

## 3. 知識點

### 3.1 `database/sql` 要連線時的三步決策

這是整件事的核心。理解它就能反推所有數字。

```
要執行 Query，跟 pool 要一條連線：

  ① pool 裡有 idle 連線嗎？
       有 → 直接拿來用。OpenConnections 不變，WaitCount 不變。
       沒有 ↓
  ② 目前 OpenConnections < MaxOpenConns 嗎？
       是 → 當場新建一條。OpenConnections += 1  ★ 唯一會增加連線數的地方
       否 ↓
  ③ 進入等待佇列。WaitCount += 1                ★★ 唯一會增加 WaitCount 的地方
```

**重要的觀念修正**：我原本以為「有人在等 → 所以該建第三條」。
因果方向是反的 —— **建連線（②）發生在排隊（③）之前，是排隊的前置條件而不是結果**。
一旦 `WaitCount` 開始跳，就代表連線早就全開好了。

另外，第 ③ 步排到的人拿到的是**別人歸還的連線，而且是手交手直接給**
（歸還時若佇列有人在等，連線直接交給第一個等待者，不經過 idle pool）。
這解釋了為什麼跑完 20 次歸還，`MaxIdleClosed` 只有 1。

### 3.2 `db.Stats()` 的欄位分兩類 — 這是判讀的關鍵

| 類別 | 欄位 | 特性 |
|---|---|---|
| **瞬時值 gauge** | `OpenConnections`、`InUse`、`Idle` | 只反映取樣那一刻。沒抓到就是沒抓到。 |
| **累計值 counter** | `WaitCount`、`WaitDuration`、`MaxIdleClosed`、`MaxIdleTimeClosed`、`MaxLifetimeClosed` | 從 `sql.Open` 起只增不減，**永不歸零**。會替你記住整場發生過的事。 |

**推論時要靠累計值，不要靠瞬時值。** 這次所有站得住腳的證據都來自累計欄位。

例如：`InUse: 0` 什麼都證明不了 —— 因為我的取樣點都落在「自己那個請求的 DB 工作之外」，
自己的連線早就還回去了，當然是 0。

反過來，累計值的威力：`WaitCount: 0` 是**鐵證**，因為它永不歸零 ——
它是 0 就代表整場下來一次排隊都沒有，不是我取樣錯過了。

### 3.3 用 channel 做起跑閘門（barrier）

```go
start := make(chan struct{})

for i := 0; i < 20; i++ {
    wg.Add(1)
    go func(n int) {
        defer wg.Done()
        req, _ := http.NewRequest(...)   // 前置工作先做完
        <-start                          // ← 20 個 goroutine 全部卡在這
        http.DefaultClient.Do(req)
    }(i)
}
close(start)                             // ← 開槍
```

- **`make(chan struct{})`** — `struct{}` 是空 struct，`unsafe.Sizeof` 是 **0**。
  它傳不了資料，存在的唯一目的是**傳「時機」**。
  Go 慣例：傳資料用 `chan int`、`chan Order`；只傳訊號用 `chan struct{}`。

- **`<-start`** — 這是**接收**，不是「印出」或「執行裡面的東西」。
  語意是：拿一個值出來；**如果現在沒東西可拿，就把這個 goroutine 掛起來什麼都不做**。
  這個「卡住」就是重點 —— 像賽跑選手蹲在起跑線等槍聲。

  ```go
  ch := make(chan struct{})
  fmt.Println("A")
  <-ch                  // 永遠卡在這
  fmt.Println("B")      // 印不到
  // 輸出 A，然後 panic: all goroutines are asleep - deadlock!
  ```

- **為什麼用 `close` 而不是送值** — 這是「不用算數量」的原因：

  | 做法 | 行為 | 問題 |
  |---|---|---|
  | `start <- struct{}{}` × 20 | 點對點，一次喚醒一個 | 要知道剛好 20 個接收者；而且是**排隊一個一個叫**，又把出發時間拉開了 |
  | `close(start)` | **廣播**，所有等待者同一瞬間被喚醒 | 幾個接收者都是同一行，不用改 |

  規則：**channel 一旦 close，所有等待接收的 goroutine 立刻同時被喚醒，
  收到該型別的零值，之後任何接收都不再阻塞。**

  ```go
  ch := make(chan int)
  close(ch)
  v, ok := <-ch
  fmt.Println(v, ok)   // 0 false   ← 0 是零值，ok=false 代表 channel 已關
  ```

  所以這裡的 `close` 不是「關掉資源」，而是**開槍**。

- **`close(start)` 必須在迴圈後面** —— `go func(){}()` 是立刻返回的（只負責啟動，不等它跑完），
  所以迴圈跑完時 20 個 goroutine 都已誕生並卡在 `<-start`。
  若 close 寫在迴圈內，第 1 個會馬上出發而第 20 個還沒被建立，barrier 就白做了。

- **`WaitGroup` 和 barrier 是同一種東西的兩個方向**：
  `wg.Wait()` 是「等 N 件事做完」，`<-start` 是「讓 N 件事同時開始」。一個管收尾，一個管起跑。

### 3.4 `QueryRow(...).Scan(...)` — `Scan` 的真正職責是「歸還連線」

```go
if err := db.QueryRow("SELECT pg_sleep(1)").Scan(&s); err != nil { ... }
```

`Scan` 做兩件事：

**(a) 錯誤只在 `Scan` 才浮出來。**
`func (db *DB) QueryRow(...) *Row` —— 注意它**沒有回傳 error**。
錯誤被存在 `*Row` 的私有欄位裡，`Scan` 時才回傳：

```go
func (r *Row) Scan(dest ...any) error {
    if r.err != nil { return r.err }   // ← 查詢階段的錯誤在這裡才浮出來
    ...
}
```

所以 SQL 打錯字又不 `Scan`，程式會**完全靜默地「成功」**。
（對照 `db.Query` 是 `rows, err := db.Query(...)`，當場給你 error。）

**(b) 連線在 `Scan` 結束時才被還回 pool。** ← 更關鍵

`QueryRow` 被呼叫的那一刻查詢就已送出，並且它從 pool 拿了一條連線包在 `*Row` 裡握著。
歸還寫在 `Scan` 的最後一行：

```go
func (r *Row) Scan(dest ...any) error {
    // ... 取值 ...
    return r.rows.Close()     // ★ 這裡才把連線放回 pool
}
```

| 寫法 | 連線何時歸還 |
|---|---|
| `db.QueryRow(...).Scan(&s)` | `Scan` 結束時，立刻 |
| `db.QueryRow(...)` 不接 `Scan` | **永遠不還**（只能靠 GC finalizer，不可靠也不及時）|

在這個測試裡少了 `Scan` 是致命的：每個請求吃掉一條連線不還，3 條用完後
**所有後續請求永遠卡在等待佇列**，`wg.Wait()` 不會回來。

`pg_sleep` 回傳 `void`，變數 `s` 其實用不到 —— 但 `Scan` 這個呼叫必須存在。
語意更清楚的替代寫法：

```go
if _, err := db.Exec("SELECT pg_sleep(1)"); err != nil { ... }
```

`Exec` 沒有 result set 要管，執行完自己歸還連線，而且直接回傳 error。

---

## 4. 推理過程（最值得留下的部分）

### 4.1 先確定每一行輸出的「作者」和「時間點」

`fetchPoolStats` 的流程是「發請求 → 等回應 → ReadAll → Printf」，
所以 **`Printf` 一定發生在 HTTP 往返完成之後**。
又因為 `fmt.Printf` 對 stdout 有鎖（不會交錯成亂碼，而是照呼叫先後排成一行行），得到：

> **輸出的先後順序 = 「HTTP 往返完成」的先後順序。**

這是後面所有推論的基礎。

| 輸出標籤 | 誰印的 | 印出那刻代表什麼已完成 |
|---|---|---|
| `[0]`~`[19]` `after` | 20 個 goroutine 各自 | 該 goroutine 的業務請求**和**它接著打的 `/debug/pool` 都已收到回應 |
| `[-1] mid` | 主 goroutine | `close(start)` 之後那次 `/debug/pool` 已收到回應 |
| `[-1] final` | 主 goroutine | `wg.Wait()` 已通過，20 個 goroutine 全部結束 |

### 4.2 從順序推論（軟證據）

`[0] pool(after)` 出現在 `[-1] pool(mid)` **之前**。工作量對比：

```
主 goroutine:   t0 ─► 1 次往返 (P) ─► 印 "mid"
goroutine 0:    t0 ─► 業務請求 (Q) ─► 再 1 次往返 (P') ─► 印 "after"
```

`/debug/pool` 的 handler 只做 `db.Stats()`（讀記憶體計數器，**完全不碰 DB**），所以 P ≈ P'。代入：

```
Q + P' < P   →   Q + P < P   →   Q < 0  ?!
```

Q 不可能是負的，所以這個矛盾說明 **Q 小到已被雜訊淹沒** ——
業務請求的耗時並沒有大於「本機 HTTP 往返之間的隨機抖動」。

**這條推論的限制（要誠實）**：有三個雜訊我無法從輸出排除 ——
goroutine 排程順序、主 goroutine 自己也在搶 CPU、TCP 連線重用與否成本不同。
所以它只是暗示，不是證明。真正的鐵證在下面。

### 4.3 從累計欄位推論（硬證據）

三個 `Closed` 欄位全為 0：

```json
"MaxIdleClosed": 0, "MaxIdleTimeClosed": 0, "MaxLifetimeClosed": 0
```

它們是累計值 → **整場沒有任何連線被關閉過** → **`OpenConnections` 單調不減**。

既然最後是 1，那它從頭到尾就一直是 1，中間不可能偷偷變 2 又降回來。

（那個 1 是 `database.New()` 裡 `db.Ping()` 建的，程式啟動就在了。）

一條連線同一時間只能跑一個查詢，所以：

> **從來沒有任何兩個查詢在時間上重疊過。**

20 個請求是完全排隊接力通過那一條連線 —— 走的是決策流程 ①（拿 idle），**不算 `WaitCount`**。
連第 2 條都沒開，離 ③ 的門檻（3 條全滿）差得遠。

所以 `WaitCount: 0` **在邏輯上是必然的**。

### 4.4 完整推論鏈

```
三個 Closed 欄位全為 0
        ↓
連線從未被關閉 ⇒ OpenConnections 單調不減
        ↓
最終 OpenConnections = 1 ⇒ 全程只存在 1 條連線
        ↓
1 條連線一次只能跑 1 個查詢 ⇒ 20 個查詢完全沒有時間重疊
        ↓
從未觸發「3 條全滿」⇒ WaitCount 必然為 0
        ↓
（成因）查詢佔用連線的時間 ≪ 請求之間的抵達時間差
        ↑
        由「[0] after 比 [-1] mid 早印出」旁證
```

**修 barrier 修不好這件事** —— barrier 只能對齊「出發時間」，
管不了「請求在伺服器內部走完前置流程的時間差」，更管不了「查詢本身太短」。

### 4.5 方法論（可以帶到別的問題）

1. **先問「這個數字是瞬時值還是累計值」**，再決定它能不能當證據。
2. **累計值 + 單調性 = 可以做反證法**。「三個 Closed 全 0 ⇒ 單調不減 ⇒ 最終值就是全程最大值」
   這個推法不需要任何時間資訊，最穩。
3. **瞬時值為 0 通常只代表取樣點不對**，不代表事情沒發生。
4. **用算術交叉驗證**（見 5.3）。如果理論值算得出來，就能直接排除錯誤假設。
5. **誠實區分「量到的」和「估的」**。「0.3ms / 1–5ms」是量級經驗值，不是從輸出量到的。
   要真數字得自己量（`time.Since`，或 psql `\timing`）。

---

## 5. 實驗紀錄

### 5.1 原始症狀（快查詢，`/api/v1/orders`）

```
[0..19] pool(after) {"MaxOpenConnections":3,"OpenConnections":1,"InUse":0,"Idle":1,
                     "WaitCount":0,"WaitDuration":0,"MaxIdleClosed":0,...}
[-1] pool(mid)   同上
[-1] pool(final) 同上
```

加了 barrier（`close(start)`）之後 **完全沒改善** —— 印證 4.4 的結論。

### 5.2 加上 `pg_sleep(1)` 之後

在 `main.go` 加測試用端點：

```go
r.GET("/debug/slow", func(c *gin.Context){
    var s string
    if err := db.QueryRow("SELECT pg_sleep(1)").Scan(&s); err != nil {
        c.JSON(500, gin.H{"error": err.Error()})
        return
    }
    c.JSON(200, gin.H{"ok": true})
})
```

輸出：

```
[-1] pool(mid)   {"OpenConnections":2,"InUse":2,"Idle":0,"WaitCount":0,"WaitDuration":0,
                  "MaxIdleClosed":0,...}
[-1] pool(final) {"OpenConnections":2,"InUse":0,"Idle":2,"WaitCount":17,
                  "WaitDuration":57509471322,"MaxIdleClosed":1,...}
```

### 5.3 「為什麼 `OpenConnections` 只有 2？」— 其實開到 3 了

`final` 的 2 是「結束後剩下的」，不是「最多開過的」。三個證據：

**證據一：`WaitCount: 17` 直接說了。**
`20 - 17 = 3` 個請求「拿到連線時完全沒等」，代表同時有 3 條連線可用。
若只有 2 條，第 3 個請求就會排隊，`WaitCount` 會是 **18**。

**證據二：`MaxIdleClosed: 1` 是第 3 條存在過的收據。**
這欄位的定義是「連線歸還時因 idle pool 已滿（`SetMaxIdleConns(2)`）而被關閉」的次數。
沒有第 3 條，這個數字不可能是 1。對帳：

```
曾經開過 3 條  −  被關 1 條 (MaxIdleClosed)  =  final 2 條   ✓
```

**證據三：`WaitDuration` 的算術。** 3 條連線 / 20 請求 / 每個 1 秒：

| 時段 | 誰在跑 | 這批各等了 |
|---|---|---|
| 0–1s | 1,2,3 | 0s |
| 1–2s | 4,5,6 | 1s |
| 2–3s | 7,8,9 | 2s |
| 3–4s | 10,11,12 | 3s |
| 4–5s | 13,14,15 | 4s |
| 5–6s | 16,17,18 | 5s |
| 6–7s | 19,20 | 6s |

```
WaitCount    = 20 − 3 = 17                            ✓ 完全吻合
WaitDuration = 3×(1+2+3+4+5) + 2×6 = 45 + 12 = 57s    ✓ 實測 57.51s（0.51s 是雜訊）
```

假設只有 2 條的話：`WaitCount = 18`、`WaitDuration = 2×(1+…+9) = 90s`。
90s vs 實測 57.5s 差一大截 —— **算術完全排除「只有 2 條」**。

**那 `mid` 為什麼看到 2？** 關鍵線索是 `mid` 那行的 `WaitCount: 0` ——
累計值還是 0，代表那一刻一次排隊都還沒發生；配合 `InUse: 2`，
只有一個解釋：**那一刻 20 個請求裡只有 2 個真正走到 DB 層**。

因為 `close(start)` 到 `mid` 拿到回應只隔了幾毫秒（一次本機往返），
那幾毫秒裡其他 goroutine 還在建 TCP、送 header、過 middleware。
第 3 條連線是再過幾毫秒才建的，快照剛好卡在中間 —— 又是瞬時值的陷阱。

**`MaxIdleClosed` 為什麼只有 1？** 因為歸還時若佇列有人在等，連線是手交手直接給下一位，
不經過 idle pool。只有最後一批（19,20）結束、佇列空了，連線才真正回到 idle pool，
此時第 3 條擠不進上限 2 的 idle pool → 被關 → `MaxIdleClosed = 1`。

### 5.4 想在快照裡真的看到 `OpenConnections: 3`

把取樣延後 / 連續取樣：

```go
close(start)
for i := 0; i < 6; i++ {
    time.Sleep(1 * time.Second)
    fetchPoolStats(-1, fmt.Sprintf("t+%ds", i+1))
}
wg.Wait()
fetchPoolStats(-1, "final")
```

預期 `OpenConnections: 3, InUse: 3, Idle: 0`，`WaitCount` 每秒往上爬（上限 17）。

---

## 6. 未追完的線索

- **`SetConnMaxLifetime(50 * time.Second)` 什麼情況下會咬人？**
  這次跑 7 秒，所以 `MaxLifetimeClosed` 是 0，完全沒觸發。
  什麼場景下連線壽命到期會造成問題（例如長交易、或 lifetime 設太短導致頻繁重建）？

- **`SetMaxIdleConns(2)` 該設多少？** 這次看到第 3 條被關掉（`MaxIdleClosed: 1`）。
  在真實流量下反覆「建了又關」的成本值得量一次。

- **量真正的數字**：`ListOrderByUserID` 實際佔用連線多久？請求之間的抵達時間差實際多大？
  這次全靠邏輯反推，沒有絕對時間。用 `time.Since` 或 psql `\timing` 補上。

- **`test-case/` 裡的 `/debug/slow` 和 `/debug/pool` 是測試用端點**，
  正式環境要移除或加上保護（目前 `/debug/pool` 沒有掛 `AuthMiddleware`）。

Mock Go Fresh API
=================

ภาพรวม
-------
เซิร์ฟเวอร์ Go สำหรับ mock API (เก็บข้อมูลในหน่วยความจำ ไม่พึ่ง MongoDB) พร้อม HTML Console ที่ออกแบบใหม่ รองรับการใช้งานที่ง่ายและสะดวก

ข้อกำหนด
---------
- Go 1.20+ (หรือเทียบเท่า)
- **ไม่ต้องใช้ MongoDB** — ข้อมูลเก็บในหน่วยความจำของ process

> 🔴 **ข้อมูลหายเมื่อ restart / redeploy** (ยอมรับได้สำหรับ mock ที่ใช้ทำ QA)
> ถ้าต้องการข้อมูลตั้งต้นทุกครั้งที่ boot ให้ตั้ง env `SEED_SNAPSHOTS` เป็น JSON array ของ snapshot เช่น
>
> ```json
> [{"client_name":"QATEST","prefix":"QATEST","data":[
>   {"username":"qauser1","prefix":"QA1","currency":"THB","web":"QATEST",
>    "month":"08","year":"2026","betAmt":1000000,"validAmount":950000,
>    "memberWl":-250000.75,"memberComm":1200.5,"memberTotal":-248800.25}]}]
> ```

วิธีรัน
-------
```bash
go run test.go
```

จากนั้นเปิด `index.html` ใน browser หรือเข้าไปที่ `http://localhost:8080`

---

HTML Console - คุณสมบัติใหม่
------------------------------

### 🎨 อินเทอร์เฟซที่ออกแบบใหม่
- **ดีไซนทันสมัย** - ใช้ Color Palette แบบ Professional (Slate/Blue)
- **รองรับ 2 ภาษา** - สลับภาษา EN/TH ได้ทันที
- **Dark/Light Mode** - เปลี่ยนธีมได้ตามต้องการ
- **Responsive** - รองรับทุกขนาดหน้าจอ (Desktop, Tablet, Mobile)

### 📊 เมนูหลัก

#### 1. Dashboard
- แสดงสถิติสรุป: Total Records, Unique Users, Websites, Currencies
- ตารางข้อมูลล่าสุด 5 รายการ
- **Auto-refresh** ทุก 35 วินาที พร้อมตัวนับถอยหลัง

#### 2. Query Data
- ค้นหาข้อมูล Win/Lose ตามเงื่อนไข
- Dropdown เดือนแสดงภาษาไทย (ม.ค., ก.พ., ...)
- Dropdown ปี 2020-2030

#### 3. Insert Record
- **2 โหมดการกรอก**: Form Mode / JSON Mode
- Form Mode: กรอกข้อมูลผ่านฟอร์มพร้อม Dropdown ที่ใช้งานง่าย
- JSON Mode: วาง JSON โดยตรง
- **Multi-month Insert**: เลือก "All Months" เพื่อ insert ทั้ง 12 เดือนในครั้งเดียว
- Dropdown เดือน: 01 - Jan (ม.ค.), 02 - Feb (ก.พ.), ...
- Dropdown ปี: 2020-2030

#### 4. Manage Data
- ตารางแสดงข้อมูลพร้อม Pagination
- **Search/Filter** แบบ real-time
- **Table View / JSON View** สลับกันได้
- ปุ่ม Edit/Delete ในแต่ละแถว
- **Auto-refresh** ทุก 35 วินาที

#### 5. Settings
- ตั้งค่า API Base URL
- ตั้งค่า Theme (Light/Dark/System)
- บันทึกการตั้งค่าลง localStorage

### ✨ ฟีเจอร์พิเศษ

| ฟีเจอร์ | รายละเอียด |
|---------|------------|
| 🌐 Bilingual | รองรับภาษาไทยและอังกฤษ |
| 🌓 Dark Mode | สลับธีมสว่าง/มืดได้ |
| ⏱️ Auto-refresh | รีเฟรชอัตโนมัติทุก 35 วินาทีที่ Dashboard และ Manage Data |
| 🔢 Countdown | แสดงเวลานับถอยหลังก่อนรีเฟรช |
| 📅 All Months | เลือก "All Months" เพื่อ insert ทั้ง 12 เดือน |
| 🔔 Toast Notifications | แจ้งเตือนแบบ non-blocking |
| 📝 Form/JSON Mode | สลับโหมดการกรอกข้อมูลได้ |
| 📱 Responsive | ใช้งานได้บนทุกอุปกรณ์ |

---

ตัวแปรแวดล้อม
--------------
- `MONGO_URI`: MongoDB connection string
- `PORT`: ถ้าไม่กำหนดจะใช้ 8080

Endpoints
---------

### 1) POST `/api/v1/ext/winloseEsByMonthMulti`
ค้นหาข้อมูล Win/Lose ตามเงื่อนไข

**Request body:**
```json
{"cur":"THB","month":"01","year":"2026","username":"user_demo","web":"WEB1"}
```

**Response body:**
```json
{
  "code": 0,
  "msg": "SUCCESS",
  "data": {
    "username": "user_demo",
    "prefix": null,
    "currency": "THB",
    "betAmt": -542668096.59,
    "validAmount": -533699975.73,
    "memberWl": -1226022.9421,
    "memberComm": 0,
    "memberTotal": -1226022.9421
  }
}
```

**Response body (Suspended = true, HTTP 200):**
```json
{
  "code": 403,
  "msg": "Permission denied."
}
```

### 2) GET `/api/v1/ext/snapshotAll`
ดึงข้อมูลทั้งหมดใน collection

### 3) POST `/api/v1/ext/insertSnapshot`
เพิ่มข้อมูลเข้า `test_data.snapshot`

**Request body:**
```json
{
  "code": 0,
  "msg": "SUCCESS",
  "data": {
    "username": "superadmin",
    "prefix": null,
    "currency": "USDT",
    "betAmt": -542668096.5,
    "validAmount": -533699975.73,
    "memberWl": -1226022.9421,
    "memberComm": 0,
    "memberTotal": -1226022.9421,
    "web": "WEB1",
    "month": "01",
    "year": "2026"
  }
}
```

**Response body:**
```json
{"code":0,"msg":"SUCCESS","insertedId":"..."}
```

### 4) POST `/api/v1/ext/updateSnapshot`
แก้ไขข้อมูลโดยใช้ filter และ update

**Request body:**
```json
{
  "filter": {"data.username":"user_demo","data.month":"01","data.year":"2026"},
  "update": {"data.currency":"THB"},
  "upsert": false
}
```

**curl ตัวอย่าง:**
```bash
curl -X POST http://localhost:8080/api/v1/ext/updateSnapshot ^
  -H "Content-Type: application/json" ^
  -d "{\"filter\":{\"data.username\":\"user_demo\",\"data.month\":\"01\",\"data.year\":\"2026\"},\"update\":{\"data.currency\":\"THB\"},\"upsert\":false}"
```

**Response body:**
```json
{"code":0,"msg":"SUCCESS","matched":1,"modified":1,"upserted":null}
```

### 5) POST `/api/v1/ext/deleteSnapshot`
ลบข้อมูลตาม filter

**Request body:**
```json
{"filter":{"data.username":"user_demo","data.month":"01","data.year":"2026"}}
```

**Response body:**
```json
{"code":0,"msg":"SUCCESS","deleted":1}
```

---

หมายเหตุ
---------
- เงื่อนไขค้นหารองรับ `month/year` ทั้งที่ root และใน `data`
- `data` รองรับทั้งแบบ object และ array
- CORS เปิดใช้งาน (`Access-Control-Allow-Origin: *`) สำหรับการใช้งาน local HTML
- สำหรับ production ควรจำกัด CORS ให้เฉพาะ trusted origins

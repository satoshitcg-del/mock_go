Mock Go Fresh API
=================

ภาพรวม
-------
เซิร์ฟเวอร์ Go สำหรับ mock API โดยใช้ MongoDB เป็นแหล่งข้อมูล

ข้อกำหนด
---------
- Go 1.20+ (หรือเทียบเท่า)
- ตั้งค่า `MONGO_URI` ใน env หรือไฟล์ `.env`

วิธีรัน
-------
```bash
go run test.go
```

ตัวแปรแวดล้อม
--------------
- `MONGO_URI`: MongoDB connection string
- `PORT`: ถ้าไม่กำหนดจะใช้ 8080

Endpoints
---------
1) POST `/api/v1/ext/winloseEsByMonthMulti`
Request body:
```json
{"cur":"THB","month":"01","year":"2026","username":"user_demo","web":"WEB1"}
```
Response body (ตัวอย่าง):
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

2) GET `/api/v1/ext/snapshotAll`
ดึงข้อมูลทั้งหมดใน collection

3) POST `/api/v1/ext/insertSnapshot`
เพิ่มข้อมูลเข้า `test_data.snapshot`
Request body (ตัวอย่าง):
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
Response body (ตัวอย่าง):
```json
{"code":0,"msg":"SUCCESS","insertedId":"..."}
```

4) POST `/api/v1/ext/updateSnapshot`
แก้ไขข้อมูลใน `test_data.snapshot` โดยใช้ filter และ update
Request body (ตัวอย่าง):
```json
{
  "filter": {"data.username":"user_demo","data.month":"01","data.year":"2026"},
  "update": {"data.currency":"THB"},
  "upsert": false
}
```
ตัวอย่าง `curl`:
```bash
curl -X POST http://localhost:8080/api/v1/ext/updateSnapshot ^
  -H "Content-Type: application/json" ^
  -d "{\"filter\":{\"data.username\":\"user_demo\",\"data.month\":\"01\",\"data.year\":\"2026\"},\"update\":{\"data.currency\":\"THB\"},\"upsert\":false}"
```
Response body (ตัวอย่าง):
```json
{"code":0,"msg":"SUCCESS","matched":1,"modified":1,"upserted":null}
```

5) POST `/api/v1/ext/deleteSnapshot`
ลบข้อมูลใน `test_data.snapshot` ตาม filter
Request body (ตัวอย่าง):
```json
{"filter":{"data.username":"user_demo","data.month":"01","data.year":"2026"}}
```
Response body (ตัวอย่าง):
```json
{"code":0,"msg":"SUCCESS","deleted":1}
```

หมายเหตุ
---------
- เงื่อนไขค้นหาจะรองรับ `month/year` ทั้งที่ root และใน `data`
- `data` รองรับทั้งแบบ object และ array

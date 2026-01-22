package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
)

// Struct สำหรับรับ Request (ตาม Log)
type RequestPayload struct {
	Cur      string `json:"cur"`
	Month    string `json:"month"`
	Year     string `json:"year"`
	Username string `json:"username"`
	Web      string `json:"web"`
}

// Struct สำหรับส่ง Response กลับ (ตาม Log)
type ResponseData struct {
	Username    string  `json:"username"`
	Prefix      *string `json:"prefix"` // ใช้ *string เพราะใน log เป็น null/nil
	Currency    string  `json:"currency"`
	BetAmt      float64 `json:"betAmt"`
	ValidAmount float64 `json:"validAmount"`
	MemberWl    float64 `json:"memberWl"`
	MemberComm  float64 `json:"memberComm"`
	MemberTotal float64 `json:"memberTotal"`
}

type ResponseBody struct {
	Code int          `json:"code"`
	Msg  string       `json:"msg"`
	Data ResponseData `json:"data"`
}

func winloseHandler(w http.ResponseWriter, r *http.Request) {
	// 1. ตรวจสอบว่าเป็น POST Method หรือไม่
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// 2. อ่าน Body ที่ส่งมา (เพื่อดูว่าหน้าตาเหมือนที่คาดหวังไหม)
	body, _ := ioutil.ReadAll(r.Body)
	fmt.Printf("Received Request: %s\n", string(body))

	// 3. เตรียมข้อมูล Response (Mock Data จาก Log ของคุณ)
	mockResponse := ResponseBody{
		Code: 0,
		Msg:  "SUCCESS",
		Data: ResponseData{
			Username:    "superadmin",
			Prefix:      nil,
			Currency:    "THB",
			BetAmt:      -542668096.59,
			ValidAmount: -533699975.73,
			MemberWl:    -1226022.9421,
			MemberComm:  0,
			MemberTotal: -1226022.9421,
		},
	}

	// 4. ตั้งค่า Header และส่ง JSON กลับไป
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(mockResponse)
}

func main() {
	// สร้าง Route ให้ตรงกับ Path ใน Log
	// URL เดิม: https://api-topup.sportbookprivate.com/api/v1/ext/winloseEsByMonthMulti
	http.HandleFunc("/api/v1/ext/winloseEsByMonthMulti", winloseHandler)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	fmt.Printf("Mock Server started at port %s\n", port)
	fmt.Printf("Endpoint: http://localhost:%s/api/v1/ext/winloseEsByMonthMulti\n", port)
	
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatal(err)
	}
}

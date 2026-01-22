package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
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

type SnapshotItem struct {
	MemberComm  float64 `bson:"memberComm"`
	MemberTotal float64 `bson:"memberTotal"`
	MemberWl    float64 `bson:"memberWl"`
	Prefix      *string `bson:"prefix"`
	Username    string  `bson:"username"`
	ValidAmount float64 `bson:"validAmount"`
	BetAmt      float64 `bson:"betAmt"`
	Currency    string  `bson:"currency"`
}

type Snapshot struct {
	ClientName string         `bson:"client_name"`
	Prefix     string         `bson:"prefix"`
	Data       []SnapshotItem `bson:"data"`
}

var (
	mongoOnce   sync.Once
	mongoClient *mongo.Client
	mongoErr    error
)

func getMongoClient() (*mongo.Client, error) {
	mongoOnce.Do(func() {
		uri := os.Getenv("MONGO_URI")
		if uri == "" {
			mongoErr = fmt.Errorf("missing MONGO_URI")
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		client, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
		if err != nil {
			mongoErr = err
			return
		}

		if err := client.Ping(ctx, nil); err != nil {
			mongoErr = err
			return
		}

		mongoClient = client
	})

	return mongoClient, mongoErr
}

func winloseHandler(w http.ResponseWriter, r *http.Request) {
	// 1. ตรวจสอบว่าเป็น POST Method หรือไม่
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// 2. อ่าน Body ที่ส่งมา (เพื่อดูว่าหน้าตาเหมือนที่คาดหวังไหม)
	var req RequestPayload
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON body", http.StatusBadRequest)
		return
	}

	fmt.Printf("Received Request: %+v\n", req)

	client, err := getMongoClient()
	if err != nil {
		http.Error(w, "Database connection error", http.StatusInternalServerError)
		return
	}

	filter := bson.M{
		"month": req.Month,
		"year":  req.Year,
	}
	if req.Username != "" {
		filter["data.username"] = req.Username
	}
	if req.Cur != "" {
		filter["data.currency"] = req.Cur
	}
	if req.Web != "" {
		filter["client_name"] = req.Web
	}

	collection := client.Database("test_data").Collection("snapshot")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var snapshot Snapshot
	if err := collection.FindOne(ctx, filter).Decode(&snapshot); err != nil {
		http.Error(w, "Record not found", http.StatusNotFound)
		return
	}

	if len(snapshot.Data) == 0 {
		http.Error(w, "Record not found", http.StatusNotFound)
		return
	}

	var item *SnapshotItem
	for i := range snapshot.Data {
		candidate := &snapshot.Data[i]
		if req.Username != "" && candidate.Username != req.Username {
			continue
		}
		if req.Cur != "" && candidate.Currency != req.Cur {
			continue
		}
		item = candidate
		break
	}
	if item == nil {
		item = &snapshot.Data[0]
	}

	// 3. เตรียมข้อมูล Response (Mock Data จาก Log ของคุณ)
	mockResponse := ResponseBody{
		Code: 0,
		Msg:  "SUCCESS",
		Data: ResponseData{
			Username:    item.Username,
			Prefix:      item.Prefix,
			Currency:    item.Currency,
			BetAmt:      item.BetAmt,
			ValidAmount: item.ValidAmount,
			MemberWl:    item.MemberWl,
			MemberComm:  item.MemberComm,
			MemberTotal: item.MemberTotal,
		},
	}

	// 4. ตั้งค่า Header และส่ง JSON กลับไป
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(mockResponse)
}

func main() {
	// สร้าง Route ให้ตรงกับ Path ใน Log
	// URL เดิม: https://api-topup.sportbookprivate.com
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

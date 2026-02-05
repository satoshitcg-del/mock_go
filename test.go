package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Struct สำหรับรับ Request (ตาม Log)
type RequestPayload struct {
	Cur      string `json:"cur"`
	Currency string `json:"currency"`
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
	Web         string  `bson:"web"`
	Month       string  `bson:"month"`
	Year        string  `bson:"year"`
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

type localConfig struct {
	MongoURI string `json:"mongo_uri"`
}

func loadDotEnv(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		if key == "" {
			continue
		}
		if _, exists := os.LookupEnv(key); !exists {
			_ = os.Setenv(key, val)
		}
	}

	return nil
}

func loadMongoURI() (string, error) {
	_ = loadDotEnv(".env")

	if uri := os.Getenv("MONGO_URI"); uri != "" {
		return uri, nil
	}

	data, err := os.ReadFile("config.json")
	if err == nil {
		var cfg localConfig
		if err := json.Unmarshal(data, &cfg); err != nil {
			return "", fmt.Errorf("invalid config.json: %w", err)
		}
		if cfg.MongoURI != "" {
			return cfg.MongoURI, nil
		}
	}

	return "", fmt.Errorf("missing MONGO_URI (env or config.json)")
}

func getMongoClient() (*mongo.Client, error) {
	mongoOnce.Do(func() {
		uri, err := loadMongoURI()
		if err != nil {
			log.Printf("mongo: %v", err)
			mongoErr = err
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		client, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
		if err != nil {
			log.Printf("mongo: connect failed: %v", err)
			mongoErr = err
			return
		}

		if err := client.Ping(ctx, nil); err != nil {
			log.Printf("mongo: ping failed: %v", err)
			mongoErr = err
			return
		}

		log.Printf("mongo: connected")
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

	var conds []bson.M
	if req.Month != "" {
		// รองรับทั้ง "01" และ "1" สำหรับเดือน
		monthPatterns := []string{req.Month}
		if len(req.Month) == 1 && req.Month >= "1" && req.Month <= "9" {
			// ถ้าเป็น "1"-"9" เพิ่ม "01"-"09"
			monthPatterns = append(monthPatterns, "0"+req.Month)
		} else if len(req.Month) == 2 && req.Month[0] == '0' && req.Month[1] >= '1' && req.Month[1] <= '9' {
			// ถ้าเป็น "01"-"09" เพิ่ม "1"-"9"
			monthPatterns = append(monthPatterns, string(req.Month[1]))
		}
		
		var monthConds []bson.M
		for _, m := range monthPatterns {
			monthConds = append(monthConds, bson.M{"month": m})
			monthConds = append(monthConds, bson.M{"data.month": m})
		}
		conds = append(conds, bson.M{"$or": monthConds})
	}
	if req.Year != "" {
		conds = append(conds, bson.M{
			"$or": []bson.M{
				{"year": req.Year},
				{"data.year": req.Year},
			},
		})
	}
	if req.Username != "" {
		conds = append(conds, bson.M{"data.username": req.Username})
	}
	// รองรับทั้ง 'cur' และ 'currency' parameter
	currencyValue := req.Cur
	if currencyValue == "" {
		currencyValue = req.Currency
	}
	if currencyValue != "" {
		conds = append(conds, bson.M{"data.currency": currencyValue})
	}
	if req.Web != "" {
		conds = append(conds, bson.M{
			"$or": []bson.M{
				{"client_name": req.Web},
				{"data.web": req.Web},
			},
		})
	}

	filter := bson.M{}
	if len(conds) > 0 {
		filter["$and"] = conds
	}

	collection := client.Database("test_data").Collection("snapshot")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var raw bson.M
	if err := collection.FindOne(ctx, filter).Decode(&raw); err != nil {
		http.Error(w, "Record not found", http.StatusNotFound)
		return
	}

	rawData, ok := raw["data"]
	if !ok || rawData == nil {
		http.Error(w, "Record not found", http.StatusNotFound)
		return
	}

	var items []SnapshotItem
	switch v := rawData.(type) {
	case bson.M:
		var item SnapshotItem
		if dataBytes, err := bson.Marshal(v); err == nil {
			_ = bson.Unmarshal(dataBytes, &item)
			items = append(items, item)
		}
	case map[string]interface{}:
		var item SnapshotItem
		if dataBytes, err := bson.Marshal(v); err == nil {
			_ = bson.Unmarshal(dataBytes, &item)
			items = append(items, item)
		}
	case []interface{}:
		for _, entry := range v {
			asMap, ok := entry.(map[string]interface{})
			if !ok {
				if asBson, ok := entry.(bson.M); ok {
					asMap = asBson
				} else {
					continue
				}
			}
			var item SnapshotItem
			if dataBytes, err := bson.Marshal(asMap); err == nil {
				_ = bson.Unmarshal(dataBytes, &item)
				items = append(items, item)
			}
		}
	}

	if len(items) == 0 {
		http.Error(w, "Record not found", http.StatusNotFound)
		return
	}

	var item *SnapshotItem
	for i := range items {
		candidate := &items[i]
		if req.Username != "" && candidate.Username != req.Username {
			continue
		}
		if req.Cur != "" && candidate.Currency != req.Cur {
			continue
		}
		if req.Web != "" && candidate.Web != "" && candidate.Web != req.Web {
			continue
		}
		item = candidate
		break
	}
	if item == nil {
		item = &items[0]
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

func snapshotAllHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	client, err := getMongoClient()
	if err != nil {
		http.Error(w, "Database connection error", http.StatusInternalServerError)
		return
	}

	collection := client.Database("test_data").Collection("snapshot")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cursor, err := collection.Find(ctx, bson.M{})
	if err != nil {
		http.Error(w, "Query failed", http.StatusInternalServerError)
		return
	}
	defer cursor.Close(ctx)

	var snapshots []bson.M
	if err := cursor.All(ctx, &snapshots); err != nil {
		http.Error(w, "Decode failed", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(snapshots)
}

func insertSnapshotHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var doc bson.M
	if err := json.NewDecoder(r.Body).Decode(&doc); err != nil {
		http.Error(w, "Invalid JSON body", http.StatusBadRequest)
		return
	}
	if len(doc) == 0 {
		http.Error(w, "Empty body", http.StatusBadRequest)
		return
	}

	client, err := getMongoClient()
	if err != nil {
		http.Error(w, "Database connection error", http.StatusInternalServerError)
		return
	}

	collection := client.Database("test_data").Collection("snapshot")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	res, err := collection.InsertOne(ctx, doc)
	if err != nil {
		http.Error(w, "Insert failed", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(bson.M{
		"code":       0,
		"msg":        "SUCCESS",
		"insertedId": res.InsertedID,
	})
}

type modifyRequest struct {
	Filter bson.M `json:"filter"`
	Update bson.M `json:"update"`
	Upsert bool   `json:"upsert"`
}

type deleteRequest struct {
	Filter bson.M `json:"filter"`
}

func normalizeFilter(filter bson.M) bson.M {
	if filter == nil {
		return bson.M{}
	}
	if idVal, ok := filter["_id"]; ok {
		switch v := idVal.(type) {
		case string:
			if oid, err := primitive.ObjectIDFromHex(v); err == nil {
				filter["_id"] = oid
			}
		case map[string]interface{}:
			if hex, ok := v["$oid"].(string); ok {
				if oid, err := primitive.ObjectIDFromHex(hex); err == nil {
					filter["_id"] = oid
				}
			}
		}
	}
	return filter
}

func updateSnapshotHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req modifyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON body", http.StatusBadRequest)
		return
	}
	if len(req.Filter) == 0 || len(req.Update) == 0 {
		http.Error(w, "Missing filter or update", http.StatusBadRequest)
		return
	}

	client, err := getMongoClient()
	if err != nil {
		http.Error(w, "Database connection error", http.StatusInternalServerError)
		return
	}

	filter := normalizeFilter(req.Filter)
	update := bson.M{"$set": req.Update}

	collection := client.Database("test_data").Collection("snapshot")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	res, err := collection.UpdateOne(ctx, filter, update, options.Update().SetUpsert(req.Upsert))
	if err != nil {
		http.Error(w, "Update failed", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(bson.M{
		"code":      0,
		"msg":       "SUCCESS",
		"matched":   res.MatchedCount,
		"modified":  res.ModifiedCount,
		"upserted":  res.UpsertedID,
	})
}

func deleteSnapshotHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req deleteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON body", http.StatusBadRequest)
		return
	}
	if len(req.Filter) == 0 {
		http.Error(w, "Missing filter", http.StatusBadRequest)
		return
	}

	client, err := getMongoClient()
	if err != nil {
		http.Error(w, "Database connection error", http.StatusInternalServerError)
		return
	}

	filter := normalizeFilter(req.Filter)

	collection := client.Database("test_data").Collection("snapshot")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	res, err := collection.DeleteOne(ctx, filter)
	if err != nil {
		http.Error(w, "Delete failed", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(bson.M{
		"code":    0,
		"msg":     "SUCCESS",
		"deleted": res.DeletedCount,
	})
}

func withCORS(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next(w, r)
	}
}

func main() {
	// สร้าง Route ให้ตรงกับ Path ใน Log
	// URL เดิม: https://api-topup.sportbookprivate.com
	http.HandleFunc("/api/v1/ext/winloseEsByMonthMulti", withCORS(winloseHandler))
	http.HandleFunc("/api/v1/ext/snapshotAll", withCORS(snapshotAllHandler))
	http.HandleFunc("/api/v1/ext/insertSnapshot", withCORS(insertSnapshotHandler))
	http.HandleFunc("/api/v1/ext/updateSnapshot", withCORS(updateSnapshotHandler))
	http.HandleFunc("/api/v1/ext/deleteSnapshot", withCORS(deleteSnapshotHandler))

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	fmt.Printf("Mock Server started at port %s\n", port)
	fmt.Printf("Endpoint: http://localhost:%s/api/v1/ext/winloseEsByMonthMulti\n", port)
	fmt.Printf("Endpoint: http://localhost:%s/api/v1/ext/snapshotAll\n", port)
	fmt.Printf("Endpoint: http://localhost:%s/api/v1/ext/insertSnapshot\n", port)
	fmt.Printf("Endpoint: http://localhost:%s/api/v1/ext/updateSnapshot\n", port)
	fmt.Printf("Endpoint: http://localhost:%s/api/v1/ext/deleteSnapshot\n", port)

	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatal(err)
	}
}

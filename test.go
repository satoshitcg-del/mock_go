package main

// Mock Go — winlose mock API แบบ in-memory (ไม่พึ่ง MongoDB)
//
// เดิม handler ทุกตัวอ่าน/เขียน MongoDB `test_data.snapshot` แต่ตอน deploy จริงต่อ DB ไม่ได้
// (commit 92dfead6 จึงตัด DB path ของ winlose ทิ้งแล้ว hardcode ให้ตอบ 0 เสมอ)
// ไฟล์นี้เปลี่ยนแหล่งเก็บเป็น map ในหน่วยความจำแทน — สัญญา JSON ของทุก endpoint เหมือนเดิม
//
// 🔴 ข้อมูลอยู่ในหน่วยความจำ ⇒ **หายเมื่อ restart/redeploy** (ยอมรับได้สำหรับ mock ที่ใช้ทำ QA)
//    ถ้าต้องการให้ข้อมูลอยู่ถาวร ให้ตั้ง SEED_SNAPSHOTS เป็น JSON array ของ snapshot
//    แล้วเซิร์ฟเวอร์จะโหลดตอน start ทุกครั้ง

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
)

// ── สัญญาเดิม ห้ามเปลี่ยนรูปร่าง ────────────────────────────────────────────────

// RequestPayload — body ที่ BO ส่งมาที่ /winloseEsByMonthMulti
type RequestPayload struct {
	Cur      string `json:"cur"`
	Currency string `json:"currency"`
	Month    string `json:"month"`
	Year     string `json:"year"`
	Username string `json:"username"`
	Web      string `json:"web"`
}

// ResponseData — ก้อน data ที่ตอบกลับ
type ResponseData struct {
	Username    string  `json:"username"`
	Prefix      *string `json:"prefix"`
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

// ── ที่เก็บข้อมูลในหน่วยความจำ ─────────────────────────────────────────────────

type store struct {
	mu   sync.RWMutex
	docs map[string]map[string]interface{} // _id (hex) -> document
	seq  []string                          // เก็บลำดับที่ insert เพื่อให้ snapshotAll คืนค่าคงที่
}

var db = &store{docs: map[string]map[string]interface{}{}}

func newID() string {
	buf := make([]byte, 12)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("%024x", len(db.docs)+1)
	}
	return hex.EncodeToString(buf)
}

func (s *store) insert(doc map[string]interface{}) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	id, _ := doc["_id"].(string)
	if id == "" {
		id = newID()
	}
	doc["_id"] = id
	if _, exists := s.docs[id]; !exists {
		s.seq = append(s.seq, id)
	}
	s.docs[id] = doc
	return id
}

func (s *store) all() []map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]map[string]interface{}, 0, len(s.docs))
	for _, id := range s.seq {
		if doc, ok := s.docs[id]; ok {
			out = append(out, doc)
		}
	}
	return out
}

// findOne คืน document แรกที่ตรง filter (เลียนแบบ collection.FindOne)
func (s *store) findOne(filter map[string]interface{}) map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, id := range s.seq {
		doc, ok := s.docs[id]
		if ok && matches(doc, filter) {
			return doc
		}
	}
	return nil
}

func (s *store) updateOne(filter, update map[string]interface{}, upsert bool) (matched, modified int, upsertedID interface{}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, id := range s.seq {
		doc, ok := s.docs[id]
		if !ok || !matches(doc, filter) {
			continue
		}
		for k, v := range update {
			doc[k] = v
		}
		doc["_id"] = id
		s.docs[id] = doc
		return 1, 1, nil
	}
	if !upsert {
		return 0, 0, nil
	}
	doc := map[string]interface{}{}
	for k, v := range filter {
		if !strings.HasPrefix(k, "$") && !strings.Contains(k, ".") {
			doc[k] = v
		}
	}
	for k, v := range update {
		doc[k] = v
	}
	id, _ := doc["_id"].(string)
	if id == "" {
		id = newID()
	}
	doc["_id"] = id
	s.docs[id] = doc
	s.seq = append(s.seq, id)
	return 0, 0, id
}

func (s *store) deleteOne(filter map[string]interface{}) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, id := range s.seq {
		doc, ok := s.docs[id]
		if !ok || !matches(doc, filter) {
			continue
		}
		delete(s.docs, id)
		s.seq = append(s.seq[:i], s.seq[i+1:]...)
		return 1
	}
	return 0
}

// ── การจับคู่ filter (รองรับเท่าที่ Mongo query เดิมใช้จริง) ─────────────────────
//
// รองรับ: $and · $or · เท่ากันตรง ๆ · path แบบจุด เช่น "data.username"
// ถ้า path วิ่งผ่าน array (เช่น data เป็น []) จะถือว่าตรงเมื่อมีสมาชิกตัวใดตัวหนึ่งตรง

func matches(doc map[string]interface{}, filter map[string]interface{}) bool {
	for key, want := range filter {
		switch key {
		case "$and":
			list, ok := want.([]interface{})
			if !ok {
				return false
			}
			for _, sub := range list {
				subMap, ok := toMap(sub)
				if !ok || !matches(doc, subMap) {
					return false
				}
			}
		case "$or":
			list, ok := want.([]interface{})
			if !ok {
				return false
			}
			any := false
			for _, sub := range list {
				subMap, ok := toMap(sub)
				if ok && matches(doc, subMap) {
					any = true
					break
				}
			}
			if !any {
				return false
			}
		default:
			if !pathHasValue(doc, strings.Split(key, "."), want) {
				return false
			}
		}
	}
	return true
}

func toMap(v interface{}) (map[string]interface{}, bool) {
	m, ok := v.(map[string]interface{})
	return m, ok
}

func pathHasValue(node interface{}, path []string, want interface{}) bool {
	if len(path) == 0 {
		return sameValue(node, want)
	}
	switch typed := node.(type) {
	case map[string]interface{}:
		child, ok := typed[path[0]]
		if !ok {
			return false
		}
		return pathHasValue(child, path[1:], want)
	case []interface{}:
		for _, entry := range typed {
			if pathHasValue(entry, path, want) {
				return true
			}
		}
		return false
	default:
		return false
	}
}

func sameValue(a, want interface{}) bool {
	if a == nil || want == nil {
		return a == want
	}
	af, aIsNum := asFloat(a)
	wf, wIsNum := asFloat(want)
	if aIsNum && wIsNum {
		return af == wf
	}
	return fmt.Sprintf("%v", a) == fmt.Sprintf("%v", want)
}

func asFloat(v interface{}) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int32:
		return float64(n), true
	case int64:
		return float64(n), true
	}
	return 0, false
}

// ── helper ที่ยกมาจากเวอร์ชันเดิม ──────────────────────────────────────────────

func getSuspendedFlag(m map[string]interface{}) (bool, bool) {
	for k, v := range m {
		if strings.ToLower(strings.TrimSpace(k)) != "suspended" {
			continue
		}
		switch val := v.(type) {
		case bool:
			return val, true
		case string:
			lv := strings.ToLower(strings.TrimSpace(val))
			if lv == "true" || lv == "1" || lv == "yes" {
				return true, true
			}
			if lv == "false" || lv == "0" || lv == "no" {
				return false, true
			}
		case float64:
			return val != 0, true
		}
	}
	return false, false
}

func str(m map[string]interface{}, key string) string {
	if v, ok := m[key]; ok && v != nil {
		return fmt.Sprintf("%v", v)
	}
	return ""
}

func num(m map[string]interface{}, key string) float64 {
	if v, ok := m[key]; ok {
		if f, isNum := asFloat(v); isNum {
			return f
		}
	}
	return 0
}

func prefixPtr(m map[string]interface{}) *string {
	v, ok := m["prefix"]
	if !ok || v == nil {
		return nil
	}
	s := fmt.Sprintf("%v", v)
	return &s
}

// ── handlers ──────────────────────────────────────────────────────────────────

func writeJSON(w http.ResponseWriter, status int, body interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func emptyWinlose(username, currency string) ResponseBody {
	return ResponseBody{Code: 0, Msg: "SUCCESS", Data: ResponseData{
		Username: username, Prefix: nil, Currency: currency,
	}}
}

func winloseHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req RequestPayload
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON body", http.StatusBadRequest)
		return
	}
	fmt.Printf("Received Request: %+v\n", req)

	// รองรับทั้ง 'cur' และ 'currency' — ค่าเริ่มต้น THB (พฤติกรรมเดิม)
	currency := req.Cur
	if currency == "" {
		currency = req.Currency
	}
	if currency == "" {
		currency = "THB"
	}
	fallbackUsername := req.Username
	if fallbackUsername == "" {
		fallbackUsername = "superadmin"
	}

	filter := buildWinloseFilter(req, currency)
	doc := db.findOne(filter)
	if doc == nil {
		writeJSON(w, http.StatusOK, emptyWinlose(fallbackUsername, currency))
		return
	}

	candidates := collectCandidates(doc["data"])
	if len(candidates) == 0 {
		writeJSON(w, http.StatusOK, emptyWinlose(fallbackUsername, currency))
		return
	}

	selected := candidates[0]
	for _, candidate := range candidates {
		if req.Username != "" && str(candidate, "username") != req.Username {
			continue
		}
		if req.Cur != "" && str(candidate, "currency") != req.Cur {
			continue
		}
		if web := str(candidate, "web"); req.Web != "" && web != "" && web != req.Web {
			continue
		}
		selected = candidate
		break
	}

	if suspended, has := getSuspendedFlag(selected); has && suspended {
		writeJSON(w, http.StatusOK, map[string]interface{}{"code": 403, "msg": "Permission denied."})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"code": 0, "msg": "SUCCESS", "data": buildData(selected),
	})
}

// buildData สร้างก้อน data ของ response
//
// 🔑 สินค้าแต่ละตัวใน BO อ่านคนละช่อง (`wlsnapshot_data.response`) เช่น
//    SportbookV.2 -> data.memberTotal · Amb SuperAPI -> data.memberWl · Thai Lotto -> data.invoiceAmt
// จึง **echo ทุก field ที่ผู้ใช้ใส่ไว้ใน snapshot** ออกไปด้วย เพื่อให้ mock ตัวเดียวรองรับได้ทุกสินค้า
// โดยยังคงช่องมาตรฐานเดิมไว้ครบ (คนที่อ่านช่องเดิมอยู่ไม่พัง)
func buildData(item map[string]interface{}) map[string]interface{} {
	data := map[string]interface{}{
		"username":    str(item, "username"),
		"prefix":      prefixPtr(item),
		"currency":    str(item, "currency"),
		"betAmt":      num(item, "betAmt"),
		"validAmount": num(item, "validAmount"),
		"memberWl":    num(item, "memberWl"),
		"memberComm":  num(item, "memberComm"),
		"memberTotal": num(item, "memberTotal"),
	}
	// ช่องอื่นที่ผู้ใช้ใส่มาเอง (invoiceAmt, winlose, amount, ...) ส่งต่อทั้งหมด
	for k, v := range item {
		switch k {
		case "web", "month", "year", "suspended": // ช่องที่ใช้จับคู่เท่านั้น ไม่ต้องส่งกลับ
			continue
		}
		if _, exists := data[k]; !exists {
			data[k] = v
		}
	}
	return data
}

// buildWinloseFilter สร้าง filter ชุดเดียวกับ Mongo query เดิม (commit 93538f23)
func buildWinloseFilter(req RequestPayload, currency string) map[string]interface{} {
	var conds []interface{}

	if req.Month != "" {
		months := []string{req.Month}
		// รองรับทั้ง "8" และ "08"
		if len(req.Month) == 1 && req.Month >= "1" && req.Month <= "9" {
			months = append(months, "0"+req.Month)
		} else if len(req.Month) == 2 && req.Month[0] == '0' && req.Month[1] >= '1' && req.Month[1] <= '9' {
			months = append(months, string(req.Month[1]))
		}
		var or []interface{}
		for _, m := range months {
			or = append(or, map[string]interface{}{"month": m})
			or = append(or, map[string]interface{}{"data.month": m})
		}
		conds = append(conds, map[string]interface{}{"$or": or})
	}
	if req.Year != "" {
		conds = append(conds, map[string]interface{}{"$or": []interface{}{
			map[string]interface{}{"year": req.Year},
			map[string]interface{}{"data.year": req.Year},
		}})
	}
	if req.Username != "" {
		conds = append(conds, map[string]interface{}{"data.username": req.Username})
	}
	if currency != "" {
		conds = append(conds, map[string]interface{}{"data.currency": currency})
	}
	if req.Web != "" {
		conds = append(conds, map[string]interface{}{"$or": []interface{}{
			map[string]interface{}{"client_name": req.Web},
			map[string]interface{}{"data.web": req.Web},
		}})
	}

	if len(conds) == 0 {
		return map[string]interface{}{}
	}
	return map[string]interface{}{"$and": conds}
}

// collectCandidates รองรับ data ที่เป็น object เดี่ยวหรือ array (เหมือนเดิม)
func collectCandidates(raw interface{}) []map[string]interface{} {
	switch typed := raw.(type) {
	case map[string]interface{}:
		return []map[string]interface{}{typed}
	case []interface{}:
		out := make([]map[string]interface{}, 0, len(typed))
		for _, entry := range typed {
			if m, ok := entry.(map[string]interface{}); ok {
				out = append(out, m)
			}
		}
		return out
	}
	return nil
}

func snapshotAllHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, db.all())
}

func insertSnapshotHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var doc map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&doc); err != nil {
		http.Error(w, "Invalid JSON body", http.StatusBadRequest)
		return
	}
	if len(doc) == 0 {
		http.Error(w, "Empty body", http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"code": 0, "msg": "SUCCESS", "insertedId": db.insert(doc),
	})
}

type modifyRequest struct {
	Filter map[string]interface{} `json:"filter"`
	Update map[string]interface{} `json:"update"`
	Upsert bool                   `json:"upsert"`
}

type deleteRequest struct {
	Filter map[string]interface{} `json:"filter"`
}

// normalizeFilter รองรับ _id ที่ส่งมาเป็น {"$oid": "..."} เหมือนเดิม
func normalizeFilter(filter map[string]interface{}) map[string]interface{} {
	if filter == nil {
		return map[string]interface{}{}
	}
	if idVal, ok := filter["_id"]; ok {
		if m, isMap := idVal.(map[string]interface{}); isMap {
			if hexID, ok := m["$oid"].(string); ok {
				filter["_id"] = hexID
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
	matched, modified, upserted := db.updateOne(normalizeFilter(req.Filter), req.Update, req.Upsert)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"code": 0, "msg": "SUCCESS",
		"matched": matched, "modified": modified, "upserted": upserted,
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
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"code": 0, "msg": "SUCCESS", "deleted": db.deleteOne(normalizeFilter(req.Filter)),
	})
}

// rootHandler — เดิม "/" ตอบ 404 ทำให้แยกไม่ออกว่าเซิร์ฟเวอร์ตายหรือ route ผิด
// ตอนนี้เสิร์ฟ index.html ถ้ามี ไม่งั้นตอบสถานะเป็น JSON
func rootHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	if data, err := os.ReadFile("index.html"); err == nil {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(data)
		return
	}
	paths := []string{
		"POST /api/v1/ext/winloseEsByMonthMulti",
		"GET  /api/v1/ext/snapshotAll",
		"POST /api/v1/ext/insertSnapshot",
		"POST /api/v1/ext/updateSnapshot",
		"POST /api/v1/ext/deleteSnapshot",
	}
	sort.Strings(paths)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status": "ok", "storage": "in-memory", "records": len(db.all()), "endpoints": paths,
	})
}

// loadSeed โหลดข้อมูลตั้งต้นจาก env SEED_SNAPSHOTS (JSON array ของ snapshot)
// ใช้กู้ข้อมูลหลัง redeploy โดยไม่ต้องพึ่ง DB
func loadSeed() {
	raw := strings.TrimSpace(os.Getenv("SEED_SNAPSHOTS"))
	if raw == "" {
		return
	}
	var docs []map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &docs); err != nil {
		log.Printf("SEED_SNAPSHOTS ไม่ใช่ JSON array ที่ถูกต้อง: %v", err)
		return
	}
	for _, doc := range docs {
		db.insert(doc)
	}
	log.Printf("โหลด seed จาก SEED_SNAPSHOTS แล้ว %d รายการ", len(docs))
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
	loadSeed()

	http.HandleFunc("/", withCORS(rootHandler))
	http.HandleFunc("/api/v1/ext/winloseEsByMonthMulti", withCORS(winloseHandler))
	http.HandleFunc("/api/v1/ext/snapshotAll", withCORS(snapshotAllHandler))
	http.HandleFunc("/api/v1/ext/insertSnapshot", withCORS(insertSnapshotHandler))
	http.HandleFunc("/api/v1/ext/updateSnapshot", withCORS(updateSnapshotHandler))
	http.HandleFunc("/api/v1/ext/deleteSnapshot", withCORS(deleteSnapshotHandler))

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	fmt.Printf("Mock Server started at port %s (storage: in-memory)\n", port)
	fmt.Printf("Console:  http://localhost:%s/\n", port)
	fmt.Printf("Endpoint: http://localhost:%s/api/v1/ext/winloseEsByMonthMulti\n", port)
	fmt.Println("⚠️  ข้อมูลอยู่ในหน่วยความจำ หายเมื่อ restart — ใช้ SEED_SNAPSHOTS ถ้าต้องการข้อมูลตั้งต้น")

	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatal(err)
	}
}

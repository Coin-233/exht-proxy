package main

import (
	"database/sql"
	"log"
	"sync"

	// "time"

	_ "modernc.org/sqlite"
)

var (
	db *sql.DB
	mu sync.Mutex
)

// visitRecord 用于异步记录访问的数据结构
type visitRecord struct {
	ip, fp, url, title string
}

// visitQueue 是有界缓冲队列，单个 worker goroutine 从中消费，避免 goroutine 无限增长
var visitQueue = make(chan visitRecord, 256)

func init() {
	go func() {
		for v := range visitQueue {
			RecordVisit(v.ip, v.fp, v.url, v.title)
		}
	}()
}

// EnqueueVisit 非阻塞地将访问记录投入队列。队列满时静默丢弃（不阻塞请求处理）。
func EnqueueVisit(ip, fp, url, title string) {
	select {
	case visitQueue <- visitRecord{ip, fp, url, title}:
	default:
	}
}

type HistoryItem struct {
	URL   string `json:"url"`
	Title string `json:"title"`
	Time  string `json:"time"`
}

func InitDB() {
	var err error
	db, err = sql.Open("sqlite", "./proxy_stats.db")
	if err != nil {
		log.Fatalf("无法打开数据库: %v", err)
	}

	// SQLite 不支持真正的并发写，限制为单连接避免锁争用
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0)

	schema := `
	PRAGMA journal_mode=WAL;
	PRAGMA busy_timeout=5000;
	PRAGMA synchronous=NORMAL;
	PRAGMA cache_size=-8000;
	CREATE TABLE IF NOT EXISTS users (id INTEGER PRIMARY KEY AUTOINCREMENT);
	CREATE TABLE IF NOT EXISTS user_ips (ip TEXT PRIMARY KEY, user_id INTEGER);
	CREATE TABLE IF NOT EXISTS user_fps (fp TEXT PRIMARY KEY, user_id INTEGER);
	CREATE TABLE IF NOT EXISTS history (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id INTEGER,
		url TEXT,
		title TEXT,
		visited_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(user_id, url)
	);
	`
	_, err = db.Exec(schema)
	if err != nil {
		log.Fatalf("初始化数据库表失败: %v", err)
	}
	log.Println("SQLite 数据库初始化完成")
}

func resolveUser(ip, fp string) int {
	if ip == "" && fp == "" {
		return 0
	}

	mu.Lock()
	defer mu.Unlock()

	var uidIP, uidFP int
	db.QueryRow("SELECT user_id FROM user_ips WHERE ip = ?", ip).Scan(&uidIP)
	db.QueryRow("SELECT user_id FROM user_fps WHERE fp = ?", fp).Scan(&uidFP)

	if uidIP == 0 && uidFP == 0 {
		res, _ := db.Exec("INSERT INTO users DEFAULT VALUES")
		id, _ := res.LastInsertId()
		uid := int(id)
		if ip != "" {
			db.Exec("INSERT INTO user_ips (ip, user_id) VALUES (?, ?)", ip, uid)
		}
		if fp != "" {
			db.Exec("INSERT INTO user_fps (fp, user_id) VALUES (?, ?)", fp, uid)
		}
		return uid
	}

	if uidIP != 0 && uidFP == 0 {
		if fp != "" {
			db.Exec("INSERT INTO user_fps (fp, user_id) VALUES (?, ?)", fp, uidIP)
		}
		return uidIP
	}

	if uidFP != 0 && uidIP == 0 {
		if ip != "" {
			db.Exec("INSERT INTO user_ips (ip, user_id) VALUES (?, ?)", ip, uidFP)
		}
		return uidFP
	}

	if uidIP != uidFP {
		// 将 uidFP 合并到 uidIP
		db.Exec("UPDATE user_ips SET user_id = ? WHERE user_id = ?", uidIP, uidFP)
		db.Exec("UPDATE user_fps SET user_id = ? WHERE user_id = ?", uidIP, uidFP)
		// 历史记录合并, 忽略违反 UNIQUE 约束的重复画廊
		db.Exec("UPDATE OR IGNORE history SET user_id = ? WHERE user_id = ?", uidIP, uidFP)
		db.Exec("DELETE FROM history WHERE user_id = ?", uidFP)
		db.Exec("DELETE FROM users WHERE id = ?", uidFP)
		return uidIP
	}

	return uidIP
}

// 记录用户访问画廊
func RecordVisit(ip, fp, url, title string) {
	uid := resolveUser(ip, fp)
	if uid == 0 {
		return
	}
	// 插入历史, 如果已经访问过该画廊, 则更新访问时间
	query := `
		INSERT INTO history (user_id, url, title, visited_at) 
		VALUES (?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(user_id, url) DO UPDATE SET visited_at = CURRENT_TIMESTAMP
	`
	db.Exec(query, uid, url, title)
}

// 获取用户统计信息
func GetUserStats(ip, fp string) (int, []HistoryItem) {
	uid := resolveUser(ip, fp)
	if uid == 0 {
		return 0, []HistoryItem{}
	}

	var total int
	db.QueryRow("SELECT COUNT(*) FROM history WHERE user_id = ?", uid).Scan(&total)

	rows, err := db.Query("SELECT url, title, datetime(visited_at, 'localtime') FROM history WHERE user_id = ? ORDER BY visited_at DESC LIMIT 50", uid)
	var history []HistoryItem
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var item HistoryItem
			rows.Scan(&item.URL, &item.Title, &item.Time)
			history = append(history, item)
		}
	}

	return total, history
}

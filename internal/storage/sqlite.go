// Package storage persists browsing statistics in SQLite.
package storage

import (
	"database/sql"
	"fmt"
	"log"
	"sync"

	_ "modernc.org/sqlite"
)

type Store struct {
	db         *sql.DB
	mu         sync.Mutex
	visitQueue chan visitRecord
	worker     sync.WaitGroup
	queueMu    sync.RWMutex
	closed     bool
}

type visitRecord struct {
	ip, fp, url, title string
}

func (s *Store) EnqueueVisit(ip, fp, url, title string) {
	s.queueMu.RLock()
	defer s.queueMu.RUnlock()
	if s.closed {
		return
	}
	select {
	case s.visitQueue <- visitRecord{ip, fp, url, title}:
	default:
	}
}

func (s *Store) Close() error {
	s.queueMu.Lock()
	if s.closed {
		s.queueMu.Unlock()
		return nil
	}
	s.closed = true
	close(s.visitQueue)
	s.queueMu.Unlock()
	s.worker.Wait()
	return s.db.Close()
}

type HistoryItem struct {
	URL   string `json:"url"`
	Title string `json:"title"`
	Time  string `json:"time"`
}

func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("无法打开数据库: %w", err)
	}

	// 限制为单连接避免锁争用
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
		db.Close()
		return nil, fmt.Errorf("初始化数据库表失败: %w", err)
	}
	log.Println("SQLite 数据库初始化完成")
	s := &Store{db: db, visitQueue: make(chan visitRecord, 256)}
	s.worker.Add(1)
	go func() {
		defer s.worker.Done()
		for v := range s.visitQueue {
			s.RecordVisit(v.ip, v.fp, v.url, v.title)
		}
	}()
	return s, nil
}

func (s *Store) resolveUser(ip, fp string) int {
	if ip == "" && fp == "" {
		return 0
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	var uidIP, uidFP int
	s.db.QueryRow("SELECT user_id FROM user_ips WHERE ip = ?", ip).Scan(&uidIP)
	s.db.QueryRow("SELECT user_id FROM user_fps WHERE fp = ?", fp).Scan(&uidFP)

	if uidIP == 0 && uidFP == 0 {
		res, _ := s.db.Exec("INSERT INTO users DEFAULT VALUES")
		id, _ := res.LastInsertId()
		uid := int(id)
		if ip != "" {
			s.db.Exec("INSERT INTO user_ips (ip, user_id) VALUES (?, ?)", ip, uid)
		}
		if fp != "" {
			s.db.Exec("INSERT INTO user_fps (fp, user_id) VALUES (?, ?)", fp, uid)
		}
		return uid
	}

	if uidIP != 0 && uidFP == 0 {
		if fp != "" {
			s.db.Exec("INSERT INTO user_fps (fp, user_id) VALUES (?, ?)", fp, uidIP)
		}
		return uidIP
	}

	if uidFP != 0 && uidIP == 0 {
		if ip != "" {
			s.db.Exec("INSERT INTO user_ips (ip, user_id) VALUES (?, ?)", ip, uidFP)
		}
		return uidFP
	}

	if uidIP != uidFP {
		// 将 uidFP 合并到 uidIP
		s.db.Exec("UPDATE user_ips SET user_id = ? WHERE user_id = ?", uidIP, uidFP)
		s.db.Exec("UPDATE user_fps SET user_id = ? WHERE user_id = ?", uidIP, uidFP)
		// 历史记录合并, 忽略违反 UNIQUE 约束的重复画廊
		s.db.Exec("UPDATE OR IGNORE history SET user_id = ? WHERE user_id = ?", uidIP, uidFP)
		s.db.Exec("DELETE FROM history WHERE user_id = ?", uidFP)
		s.db.Exec("DELETE FROM users WHERE id = ?", uidFP)
		return uidIP
	}

	return uidIP
}

// 记录用户访问画廊
func (s *Store) RecordVisit(ip, fp, url, title string) {
	uid := s.resolveUser(ip, fp)
	if uid == 0 {
		return
	}
	// 插入历史, 如果已经访问过该画廊, 则更新访问时间
	query := `
		INSERT INTO history (user_id, url, title, visited_at) 
		VALUES (?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(user_id, url) DO UPDATE SET visited_at = CURRENT_TIMESTAMP
	`
	s.db.Exec(query, uid, url, title)
}

// 获取用户统计信息
func (s *Store) GetUserStats(ip, fp string) (int, []HistoryItem) {
	uid := s.resolveUser(ip, fp)
	if uid == 0 {
		return 0, []HistoryItem{}
	}

	var total int
	s.db.QueryRow("SELECT COUNT(*) FROM history WHERE user_id = ?", uid).Scan(&total)

	rows, err := s.db.Query("SELECT url, title, datetime(visited_at, 'localtime') FROM history WHERE user_id = ? ORDER BY visited_at DESC LIMIT 50", uid)
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

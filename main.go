package main

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/gofrs/uuid"
	"github.com/gorilla/websocket"
	"golang.org/x/crypto/bcrypt"
	_ "modernc.org/sqlite"
)

var db *sql.DB

func initDB() {
	var err error
	db, err = sql.Open("sqlite", "./forum.db")
	if err != nil {
		log.Fatal(err)
	}
	tables := []string{
		`CREATE TABLE IF NOT EXISTS users (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			nickname   TEXT UNIQUE NOT NULL,
			email      TEXT UNIQUE NOT NULL,
			password   TEXT NOT NULL,
			age        INTEGER DEFAULT 0,
			gender     TEXT    DEFAULT '',
			first_name TEXT    DEFAULT '',
			last_name  TEXT    DEFAULT '',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS sessions (
			id         TEXT PRIMARY KEY,
			user_id    INTEGER NOT NULL,
			expires_at DATETIME NOT NULL,
			FOREIGN KEY(user_id) REFERENCES users(id)
		)`,
		`CREATE TABLE IF NOT EXISTS posts (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id    INTEGER NOT NULL,
			title      TEXT NOT NULL,
			content    TEXT NOT NULL,
			category   TEXT NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY(user_id) REFERENCES users(id)
		)`,
		`CREATE TABLE IF NOT EXISTS comments (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			post_id    INTEGER NOT NULL,
			user_id    INTEGER NOT NULL,
			content    TEXT NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY(post_id) REFERENCES posts(id),
			FOREIGN KEY(user_id) REFERENCES users(id)
		)`,
		`CREATE TABLE IF NOT EXISTS messages (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			sender_id   INTEGER NOT NULL,
			receiver_id INTEGER NOT NULL,
			content     TEXT NOT NULL,
			created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY(sender_id)   REFERENCES users(id),
			FOREIGN KEY(receiver_id) REFERENCES users(id)
		)`,
	}
	for _, q := range tables {
		if _, err := db.Exec(q); err != nil {
			log.Fatal(err)
		}
	}
	log.Println("Database ready")
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func errJSON(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

type User struct {
	ID        int    `json:"id"`
	Nickname  string `json:"nickname"`
	Email     string `json:"email"`
	Age       int    `json:"age"`
	Gender    string `json:"gender"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
}

func getUserFromSession(r *http.Request) (*User, error) {
	cookie, err := r.Cookie("session_token")
	if err != nil {
		return nil, err
	}
	var u User
	err = db.QueryRow(`
		SELECT u.id, u.nickname, u.email, u.age, u.gender, u.first_name, u.last_name
		FROM users u
		JOIN sessions s ON u.id = s.user_id
		WHERE s.id = ? AND s.expires_at > datetime('now')
	`, cookie.Value).Scan(&u.ID, &u.Nickname, &u.Email, &u.Age, &u.Gender, &u.FirstName, &u.LastName)
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func handleRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		errJSON(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	var data struct {
		Nickname  string `json:"nickname"`
		Email     string `json:"email"`
		Password  string `json:"password"`
		Age       int    `json:"age"`
		Gender    string `json:"gender"`
		FirstName string `json:"first_name"`
		LastName  string `json:"last_name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
		errJSON(w, http.StatusBadRequest, "Bad request")
		return
	}
	if data.Nickname == "" || data.Email == "" || data.Password == "" {
		errJSON(w, http.StatusBadRequest, "Nickname, email and password are required")
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(data.Password), bcrypt.DefaultCost)
	if err != nil {
		errJSON(w, http.StatusInternalServerError, "Server error")
		return
	}
	_, err = db.Exec(`
		INSERT INTO users (nickname, email, password, age, gender, first_name, last_name)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, data.Nickname, data.Email, string(hash), data.Age, data.Gender, data.FirstName, data.LastName)
	if err != nil {
		errJSON(w, http.StatusBadRequest, "Nickname or email already taken")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "Registered successfully"})
}

func handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		errJSON(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	var data struct {
		Login    string `json:"login"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
		errJSON(w, http.StatusBadRequest, "Bad request")
		return
	}
	var u User
	var hash string
	err := db.QueryRow(`
		SELECT id, nickname, email, age, gender, first_name, last_name, password
		FROM users WHERE nickname = ? OR email = ?
	`, data.Login, data.Login).Scan(
		&u.ID, &u.Nickname, &u.Email, &u.Age, &u.Gender, &u.FirstName, &u.LastName, &hash,
	)
	if err != nil {
		errJSON(w, http.StatusUnauthorized, "Invalid credentials")
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(data.Password)); err != nil {
		errJSON(w, http.StatusUnauthorized, "Invalid credentials")
		return
	}
	db.Exec(`DELETE FROM sessions WHERE user_id = ?`, u.ID)

	id, _ := uuid.NewV4()
	token := id.String()
	expires := time.Now().Add(24 * time.Hour)
	db.Exec(`INSERT INTO sessions (id, user_id, expires_at) VALUES (?, ?, ?)`, token, u.ID, expires)

	http.SetCookie(w, &http.Cookie{
		Name:     "session_token",
		Value:    token,
		Expires:  expires,
		Path:     "/",
		HttpOnly: true,
	})
	writeJSON(w, http.StatusOK, u)
}

func handleLogout(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("session_token")
	if err == nil {
		db.Exec(`DELETE FROM sessions WHERE id = ?`, cookie.Value)
	}
	http.SetCookie(w, &http.Cookie{
		Name:    "session_token",
		Value:   "",
		Expires: time.Now().Add(-time.Hour),
		Path:    "/",
	})
	writeJSON(w, http.StatusOK, map[string]string{"message": "Logged out"})
}

func handleMe(w http.ResponseWriter, r *http.Request) {
	u, err := getUserFromSession(r)
	if err != nil {
		errJSON(w, http.StatusUnauthorized, "Not logged in")
		return
	}
	writeJSON(w, http.StatusOK, u)
}

type Post struct {
	ID        int    `json:"id"`
	UserID    int    `json:"user_id"`
	Nickname  string `json:"nickname"`
	Title     string `json:"title"`
	Content   string `json:"content"`
	Category  string `json:"category"`
	CreatedAt string `json:"created_at"`
}

type Comment struct {
	ID        int    `json:"id"`
	PostID    int    `json:"post_id"`
	UserID    int    `json:"user_id"`
	Nickname  string `json:"nickname"`
	Content   string `json:"content"`
	CreatedAt string `json:"created_at"`
}

func handlePosts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		errJSON(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	category := r.URL.Query().Get("category")

	query := `SELECT p.id, p.user_id, u.nickname, p.title, p.content, p.category, p.created_at
		FROM posts p JOIN users u ON p.user_id = u.id`

	var rows *sql.Rows
	var err error
	if category != "" {
		rows, err = db.Query(query+" WHERE p.category = ? ORDER BY p.created_at DESC", category)
	} else {
		rows, err = db.Query(query + " ORDER BY p.created_at DESC")
	}
	if err != nil {
		errJSON(w, http.StatusInternalServerError, "DB error")
		return
	}
	defer rows.Close()

	posts := []Post{}
	for rows.Next() {
		var p Post
		rows.Scan(&p.ID, &p.UserID, &p.Nickname, &p.Title, &p.Content, &p.Category, &p.CreatedAt)
		posts = append(posts, p)
	}
	writeJSON(w, http.StatusOK, posts)
}

func handleCreatePost(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		errJSON(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	u, err := getUserFromSession(r)
	if err != nil {
		errJSON(w, http.StatusUnauthorized, "Not logged in")
		return
	}
	var data struct {
		Title    string `json:"title"`
		Content  string `json:"content"`
		Category string `json:"category"`
	}
	if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
		errJSON(w, http.StatusBadRequest, "Bad request")
		return
	}
	if data.Title == "" || data.Content == "" || data.Category == "" {
		errJSON(w, http.StatusBadRequest, "All fields required")
		return
	}
	res, err := db.Exec(`INSERT INTO posts (user_id, title, content, category) VALUES (?, ?, ?, ?)`,
		u.ID, data.Title, data.Content, data.Category)
	if err != nil {
		errJSON(w, http.StatusInternalServerError, "DB error")
		return
	}
	id, _ := res.LastInsertId()
	writeJSON(w, http.StatusOK, map[string]interface{}{"id": id})
}

func handleComments(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		errJSON(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	postID := r.URL.Query().Get("post_id")
	if postID == "" {
		errJSON(w, http.StatusBadRequest, "post_id required")
		return
	}
	rows, err := db.Query(`
		SELECT c.id, c.post_id, c.user_id, u.nickname, c.content, c.created_at
		FROM comments c JOIN users u ON c.user_id = u.id
		WHERE c.post_id = ? ORDER BY c.created_at ASC
	`, postID)
	if err != nil {
		errJSON(w, http.StatusInternalServerError, "DB error")
		return
	}
	defer rows.Close()
	comments := []Comment{}
	for rows.Next() {
		var c Comment
		rows.Scan(&c.ID, &c.PostID, &c.UserID, &c.Nickname, &c.Content, &c.CreatedAt)
		comments = append(comments, c)
	}
	writeJSON(w, http.StatusOK, comments)
}

func handleCreateComment(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		errJSON(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	u, err := getUserFromSession(r)
	if err != nil {
		errJSON(w, http.StatusUnauthorized, "Not logged in")
		return
	}
	var data struct {
		PostID  int    `json:"post_id"`
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
		errJSON(w, http.StatusBadRequest, "Bad request")
		return
	}
	if data.Content == "" || data.PostID == 0 {
		errJSON(w, http.StatusBadRequest, "All fields required")
		return
	}
	_, err = db.Exec(`INSERT INTO comments (post_id, user_id, content) VALUES (?, ?, ?)`,
		data.PostID, u.ID, data.Content)
	if err != nil {
		errJSON(w, http.StatusInternalServerError, "DB error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "Comment created"})
}

type Message struct {
	ID         int    `json:"id"`
	SenderID   int    `json:"sender_id"`
	ReceiverID int    `json:"receiver_id"`
	Nickname   string `json:"nickname"`
	Content    string `json:"content"`
	CreatedAt  string `json:"created_at"`
}

func handleMessages(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		errJSON(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	me, err := getUserFromSession(r)
	if err != nil {
		errJSON(w, http.StatusUnauthorized, "Not logged in")
		return
	}
	withID, _ := strconv.Atoi(r.URL.Query().Get("with"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	if withID == 0 {
		errJSON(w, http.StatusBadRequest, "with param required")
		return
	}
	rows, err := db.Query(`
		SELECT m.id, m.sender_id, m.receiver_id, u.nickname, m.content, m.created_at
		FROM messages m JOIN users u ON m.sender_id = u.id
		WHERE (m.sender_id = ? AND m.receiver_id = ?)
		   OR (m.sender_id = ? AND m.receiver_id = ?)
		ORDER BY m.created_at DESC
		LIMIT 10 OFFSET ?
	`, me.ID, withID, withID, me.ID, offset)
	if err != nil {
		errJSON(w, http.StatusInternalServerError, "DB error")
		return
	}
	defer rows.Close()
	msgs := []Message{}
	for rows.Next() {
		var m Message
		rows.Scan(&m.ID, &m.SenderID, &m.ReceiverID, &m.Nickname, &m.Content, &m.CreatedAt)
		msgs = append(msgs, m)
	}
	for i, j := 0, len(msgs)-1; i < j; i, j = i+1, j-1 {
		msgs[i], msgs[j] = msgs[j], msgs[i]
	}
	writeJSON(w, http.StatusOK, msgs)
}

type UserWithStatus struct {
	ID       int    `json:"id"`
	Nickname string `json:"nickname"`
	Online   bool   `json:"online"`
	LastMsg  string `json:"last_msg"`
}

func handleUsers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		errJSON(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	me, err := getUserFromSession(r)
	if err != nil {
		errJSON(w, http.StatusUnauthorized, "Not logged in")
		return
	}
	rows, err := db.Query(`
		SELECT u.id, u.nickname,
		COALESCE((
			SELECT m.created_at FROM messages m
			WHERE (m.sender_id = u.id AND m.receiver_id = ?)
			   OR (m.sender_id = ? AND m.receiver_id = u.id)
			ORDER BY m.created_at DESC LIMIT 1
		), '') as last_msg
		FROM users u WHERE u.id != ?
		ORDER BY last_msg DESC, u.nickname ASC
	`, me.ID, me.ID, me.ID)
	if err != nil {
		errJSON(w, http.StatusInternalServerError, "DB error")
		return
	}
	defer rows.Close()
	users := []UserWithStatus{}
	for rows.Next() {
		var u UserWithStatus
		rows.Scan(&u.ID, &u.Nickname, &u.LastMsg)
		users = append(users, u)
	}
	writeJSON(w, http.StatusOK, users)
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

type WSMessage struct {
	Type      string `json:"type"`
	FromID    int    `json:"from_id,omitempty"`
	FromNick  string `json:"from_nick,omitempty"`
	ToID      int    `json:"to_id,omitempty"`
	Content   string `json:"content,omitempty"`
	UserID    int    `json:"user_id,omitempty"`
	Nickname  string `json:"nickname,omitempty"`
	CreatedAt string `json:"created_at,omitempty"`
}

type Client struct {
	conn   *websocket.Conn
	send   chan []byte
	userID int
	nick   string
}

type Hub struct {
	clients    map[*Client]bool
	register   chan *Client
	unregister chan *Client
}

var hub = &Hub{
	clients:    make(map[*Client]bool),
	register:   make(chan *Client),
	unregister: make(chan *Client),
}

func (h *Hub) run() {
	for {
		select {
		case c := <-h.register:
			h.clients[c] = true
			h.broadcast(WSMessage{Type: "online", UserID: c.userID, Nickname: c.nick})

		case c := <-h.unregister:
			if _, ok := h.clients[c]; ok {
				delete(h.clients, c)
				close(c.send)
				h.broadcast(WSMessage{Type: "offline", UserID: c.userID, Nickname: c.nick})
			}
		}
	}
}

func (h *Hub) broadcast(msg WSMessage) {
	data, _ := json.Marshal(msg)
	for c := range h.clients {
		select {
		case c.send <- data:
		default:
			close(c.send)
			delete(h.clients, c)
		}
	}
}

func (h *Hub) sendTo(userID int, data []byte) {
	for c := range h.clients {
		if c.userID == userID {
			select {
			case c.send <- data:
			default:
			}
		}
	}
}

func (c *Client) readPump() {
	defer func() {
		hub.unregister <- c
		c.conn.Close()
	}()
	c.conn.SetReadLimit(4096)
	c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})
	for {
		_, raw, err := c.conn.ReadMessage()
		if err != nil {
			break
		}
		var msg WSMessage
		if err := json.Unmarshal(raw, &msg); err != nil {
			continue
		}
		if msg.Type == "typing" && msg.ToID != 0 {
			out := WSMessage{Type: "typing", FromID: c.userID, FromNick: c.nick, ToID: msg.ToID}
			data, _ := json.Marshal(out)
			hub.sendTo(msg.ToID, data)
		}

		if msg.Type == "stop_typing" && msg.ToID != 0 {
			out := WSMessage{Type: "stop_typing", FromID: c.userID, ToID: msg.ToID}
			data, _ := json.Marshal(out)
			hub.sendTo(msg.ToID, data)
		}

		if msg.Type == "chat" && msg.ToID != 0 && msg.Content != "" {
			now := time.Now().UTC().Format("2006-01-02 15:04:05")
			db.Exec(`INSERT INTO messages (sender_id, receiver_id, content, created_at) VALUES (?, ?, ?, ?)`,
				c.userID, msg.ToID, msg.Content, now)
			out := WSMessage{
				Type: "chat", FromID: c.userID, FromNick: c.nick,
				ToID: msg.ToID, Content: msg.Content, CreatedAt: now,
			}
			data, _ := json.Marshal(out)
			hub.sendTo(msg.ToID, data)
			hub.sendTo(c.userID, data)
		}
	}
}

func (c *Client) writePump() {
	ticker := time.NewTicker(50 * time.Second)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()
	for {
		select {
		case msg, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			c.conn.WriteMessage(websocket.TextMessage, msg)
		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func handleWS(w http.ResponseWriter, r *http.Request) {
	u, err := getUserFromSession(r)
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("WS upgrade error:", err)
		return
	}
	c := &Client{conn: conn, send: make(chan []byte, 256), userID: u.ID, nick: u.Nickname}
	hub.register <- c

	for existing := range hub.clients {
		if existing.userID != u.ID {
			msg := WSMessage{Type: "online", UserID: existing.userID, Nickname: existing.nick}
			data, _ := json.Marshal(msg)
			c.send <- data
		}
	}

	go c.writePump()
	go c.readPump()
}

func main() {
	initDB()
	go hub.run()

	http.Handle("/", http.FileServer(http.Dir("./static")))

	http.HandleFunc("/api/register", handleRegister)
	http.HandleFunc("/api/login", handleLogin)
	http.HandleFunc("/api/logout", handleLogout)
	http.HandleFunc("/api/me", handleMe)

	http.HandleFunc("/api/posts", handlePosts)
	http.HandleFunc("/api/posts/create", handleCreatePost)
	http.HandleFunc("/api/comments", handleComments)
	http.HandleFunc("/api/comments/create", handleCreateComment)

	http.HandleFunc("/api/messages", handleMessages)
	http.HandleFunc("/api/users", handleUsers)

	http.HandleFunc("/ws", handleWS)

	log.Println("Server running at http://localhost:8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

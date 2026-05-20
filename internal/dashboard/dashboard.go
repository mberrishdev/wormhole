package dashboard

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"
)

type Session struct {
	Token       string
	LocalAddr   string
	PublicURL   string
	StartTime   time.Time
	Connections int
}

var (
	mu       sync.Mutex
	sessions = make(map[string]*Session)
)

func EnsureRunning(addr string) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return
	}
	ln.Close()

	mux := http.NewServeMux()
	mux.HandleFunc("/", serveHome)
	mux.HandleFunc("/api/sessions", serveSessions)
	mux.HandleFunc("/api/register", handleRegister)
	mux.HandleFunc("/api/unregister", handleUnregister)
	mux.HandleFunc("/api/connect", handleConnect)
	mux.HandleFunc("/api/disconnect", handleDisconnect)
	go http.ListenAndServe(addr, mux)

	time.Sleep(30 * time.Millisecond)
}

func Register(token, localAddr, publicURL string) {
	s := Session{Token: token, LocalAddr: localAddr, PublicURL: publicURL, StartTime: time.Now()}
	body, _ := json.Marshal(s)
	http.Post("http://localhost:4040/api/register", "application/json", bytes.NewReader(body))
}

func Unregister(token string) {
	body := []byte(`{"Token":"` + token + `"}`)
	http.Post("http://localhost:4040/api/unregister", "application/json", bytes.NewReader(body))
}

func Connect(token string) {
	body := []byte(`{"Token":"` + token + `"}`)
	http.Post("http://localhost:4040/api/connect", "application/json", bytes.NewReader(body))
}

func Disconnect(token string) {
	body := []byte(`{"Token":"` + token + `"}`)
	http.Post("http://localhost:4040/api/disconnect", "application/json", bytes.NewReader(body))
}

func handleConnect(w http.ResponseWriter, r *http.Request) {
	var req struct{ Token string }
	json.NewDecoder(r.Body).Decode(&req)
	mu.Lock()
	if s, ok := sessions[req.Token]; ok {
		s.Connections++
	}
	mu.Unlock()
}

func handleDisconnect(w http.ResponseWriter, r *http.Request) {
	var req struct{ Token string }
	json.NewDecoder(r.Body).Decode(&req)
	mu.Lock()
	if s, ok := sessions[req.Token]; ok && s.Connections > 0 {
		s.Connections--
	}
	mu.Unlock()
}

func handleRegister(w http.ResponseWriter, r *http.Request) {
	var s Session
	if err := json.NewDecoder(r.Body).Decode(&s); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	mu.Lock()
	sessions[s.Token] = &s
	mu.Unlock()
}

func handleUnregister(w http.ResponseWriter, r *http.Request) {
	var req struct{ Token string }
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	mu.Lock()
	delete(sessions, req.Token)
	mu.Unlock()
}

func serveSessions(w http.ResponseWriter, r *http.Request) {
	mu.Lock()
	defer mu.Unlock()

	list := make([]*Session, 0, len(sessions))
	for _, s := range sessions {
		list = append(list, s)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(list)
}

func serveHome(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	fmt.Fprintf(w, `<!DOCTYPE html>
<html>
<head>
    <title>Wormhole Dashboard</title>
    <style>
        body { font-family: monospace; padding: 20px; background: #0d0d0d; color: #e0e0e0; }
        h1 { color: #1D9E75; }
        table { width: 100%%; border-collapse: collapse; margin-top: 20px; }
        th { text-align: left; padding: 10px; border-bottom: 1px solid #333; color: #888; }
        td { padding: 10px; border-bottom: 1px solid #1a1a1a; }
        a { color: #1D9E75; }
    </style>
</head>
<body>
    <h1>wormhole</h1>
    <table>
        <thead>
            <tr>
                <th>token</th>
                <th>local</th>
                <th>public url</th>
                <th>connections</th>
                <th>started</th>
            </tr>
        </thead>
        <tbody id="sessions"></tbody>
    </table>

    <script>
        function load() {
            fetch('/api/sessions')
                .then(r => r.json())
                .then(data => {
                    const tbody = document.getElementById('sessions')
                    tbody.innerHTML = data.map(s => {
                        const started = new Date(s.StartTime).toLocaleTimeString()
                        return '<tr>' +
                            '<td>' + s.Token + '</td>' +
                            '<td>' + s.LocalAddr + '</td>' +
                            '<td><a href="' + s.PublicURL + '" target="_blank">' + s.PublicURL + '</a></td>' +
                            '<td>' + s.Connections + '</td>' +
                            '<td>' + started + '</td>' +
                            '</tr>'
                    }).join('')
                })
        }
i
        load()
        setInterval(load, 3000)
    </script>
</body>
</html>`)
}

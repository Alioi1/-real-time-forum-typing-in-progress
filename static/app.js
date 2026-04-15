let currentUser = null;
let currentPostID = null;
let chatWithID = null;
let chatOffset = 0;
let chatLoading = false;
let ws = null;
let onlineUsers = new Set();
 
let typingTimer = null;
let isTyping = false;
let stopTypingTimers = {};
 
window.onload = async () => {
  const res = await fetch('/api/me');
  if (res.ok) {
    currentUser = await res.json();
    showLoggedIn();
  } else {
    showAuth();
  }
};
 
function switchTab(tab) {
  document.getElementById('tab-login').style.display    = tab === 'login'    ? '' : 'none';
  document.getElementById('tab-register').style.display = tab === 'register' ? '' : 'none';
  document.querySelectorAll('.tab-btn').forEach((b, i) => {
    b.classList.toggle('active', (i === 0 && tab === 'login') || (i === 1 && tab === 'register'));
  });
}
 
async function doLogin() {
  const login = document.getElementById('login-id').value.trim();
  const pass  = document.getElementById('login-pass').value;
  const err   = document.getElementById('login-err');
  err.textContent = '';
  const res = await fetch('/api/login', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ login, password: pass })
  });
  const data = await res.json();
  if (!res.ok) { err.textContent = data.error; return; }
  currentUser = data;
  showLoggedIn();
}
 
async function doRegister() {
  const err = document.getElementById('reg-err');
  const ok  = document.getElementById('reg-ok');
  err.textContent = ''; ok.textContent = '';
  const body = {
    nickname:   document.getElementById('reg-nick').value.trim(),
    email:      document.getElementById('reg-email').value.trim(),
    password:   document.getElementById('reg-pass').value,
    first_name: document.getElementById('reg-fname').value.trim(),
    last_name:  document.getElementById('reg-lname').value.trim(),
    age:        parseInt(document.getElementById('reg-age').value) || 0,
    gender:     document.getElementById('reg-gender').value,
  };
  if (!body.nickname || !body.email || !body.password) {
    err.textContent = 'Nickname, email and password are required'; return;
  }
  const res = await fetch('/api/register', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body)
  });
  const data = await res.json();
  if (!res.ok) { err.textContent = data.error; return; }
  ok.textContent = 'Registered! Please login.';
  setTimeout(() => switchTab('login'), 1200);
}
 
async function logout() {
  await fetch('/api/logout', { method: 'POST' });
  currentUser = null;
  if (ws) { ws.close(); ws = null; }
  onlineUsers.clear();
  showAuth();
}
 
function showAuth() {
  document.getElementById('view-auth').style.display = '';
  document.getElementById('view-main').style.display = 'none';
  document.getElementById('navbar').style.display    = 'none';
}
 
function showLoggedIn() {
  document.getElementById('view-auth').style.display = 'none';
  document.getElementById('view-main').style.display = '';
  document.getElementById('navbar').style.display    = '';
  document.getElementById('nav-user').textContent    = '👤 ' + currentUser.nickname;
  connectWS();
  loadUsers();
  showView('feed');
}
 
function showView(name) {
  document.getElementById('view-feed').style.display     = name === 'feed'     ? '' : 'none';
  document.getElementById('view-new-post').style.display = name === 'new-post' ? '' : 'none';
  document.getElementById('view-post').style.display     = name === 'post'     ? '' : 'none';
  if (name === 'feed') loadPosts('');
}
 
function showNewPost() {
  showView('new-post');
  document.getElementById('post-title').value   = '';
  document.getElementById('post-content').value = '';
  document.getElementById('post-err').textContent = '';
}
 
async function loadPosts(category) {
  document.querySelectorAll('.cat-btn').forEach(b => {
    b.classList.toggle('active', b.textContent.trim() === (category || 'All'));
  });
  const url = category ? `/api/posts?category=${encodeURIComponent(category)}` : '/api/posts';
  const res = await fetch(url);
  const posts = await res.json();
  const container = document.getElementById('posts-list');
  if (!posts || posts.length === 0) {
    container.innerHTML = '<p style="color:#888;margin-top:20px">No posts yet.</p>';
    return;
  }
  container.innerHTML = posts.map(p => `
    <div class="post-card" onclick="openPost(${p.id})">
      <h3>${escHtml(p.title)}</h3>
      <div class="post-meta">
        by <b>${escHtml(p.nickname)}</b>
        <span class="post-cat">${escHtml(p.category)}</span>
        &nbsp;·&nbsp; ${formatDate(p.created_at)}
      </div>
      <p>${escHtml(p.content.substring(0, 150))}${p.content.length > 150 ? '...' : ''}</p>
    </div>
  `).join('');
}
 
async function openPost(id) {
  currentPostID = id;
  showView('post');
  const res = await fetch('/api/posts');
  const posts = await res.json();
  const p = posts.find(x => x.id === id);
  if (!p) return;
  document.getElementById('post-detail').innerHTML = `
    <h2>${escHtml(p.title)}</h2>
    <div class="post-meta">by <b>${escHtml(p.nickname)}</b>
      <span class="post-cat">${escHtml(p.category)}</span>
      &nbsp;·&nbsp; ${formatDate(p.created_at)}
    </div>
    <p style="margin-top:12px;line-height:1.6">${escHtml(p.content)}</p>
  `;
  loadComments(id);
}
 
async function loadComments(postID) {
  const res = await fetch(`/api/comments?post_id=${postID}`);
  const comments = await res.json();
  const container = document.getElementById('comments-list');
  if (!comments || comments.length === 0) {
    container.innerHTML = '<p style="color:#888">No comments yet.</p>';
    return;
  }
  container.innerHTML = comments.map(c => `
    <div class="comment-item">
      <div class="post-meta"><b>${escHtml(c.nickname)}</b> · ${formatDate(c.created_at)}</div>
      <p>${escHtml(c.content)}</p>
    </div>
  `).join('');
}
 
async function submitPost() {
  const title    = document.getElementById('post-title').value.trim();
  const content  = document.getElementById('post-content').value.trim();
  const category = document.getElementById('post-cat').value;
  const err      = document.getElementById('post-err');
  err.textContent = '';
  if (!title || !content) { err.textContent = 'Title and content required'; return; }
  const res = await fetch('/api/posts/create', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ title, content, category })
  });
  const data = await res.json();
  if (!res.ok) { err.textContent = data.error; return; }
  showView('feed');
}
 
async function submitComment() {
  const content = document.getElementById('comment-text').value.trim();
  if (!content) return;
  const res = await fetch('/api/comments/create', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ post_id: currentPostID, content })
  });
  if (res.ok) {
    document.getElementById('comment-text').value = '';
    loadComments(currentPostID);
  }
}
 
async function loadUsers() {
  const res = await fetch('/api/users');
  if (!res.ok) return;
  const users = await res.json();
  renderUsers(users);
}
 
function renderUsers(users) {
  const container = document.getElementById('users-list');
  if (!users || users.length === 0) {
    container.innerHTML = '<p style="color:#666;padding:12px">No other users</p>';
    return;
  }
  container.innerHTML = users.map(u => `
    <div class="user-item" onclick="openChat(${u.id}, '${escAttr(u.nickname)}')">
      <span class="user-dot ${onlineUsers.has(u.id) ? 'online' : ''}"></span>
      <span class="user-name">${escHtml(u.nickname)}</span>
    </div>
  `).join('');
}
 
async function refreshUsers() {
  const res = await fetch('/api/users');
  if (!res.ok) return;
  const users = await res.json();
  renderUsers(users);
}
 
function openChat(userID, nickname) {
  onStopTyping();
  document.getElementById('typing-indicator').style.display = 'none';
  chatWithID = userID;
  chatOffset = 0;
  document.getElementById('chat-with-name').textContent = nickname;
  document.getElementById('chat-box').style.display = '';
  document.getElementById('chat-messages').innerHTML = '';
  loadMessages(true);
}
 
function closeChat() {
  onStopTyping();
  document.getElementById('chat-box').style.display = 'none';
  document.getElementById('typing-indicator').style.display = 'none';
  chatWithID = null;
}
 
async function loadMessages(scrollToBottom) {
  if (!chatWithID || chatLoading) return;
  chatLoading = true;
  const res = await fetch(`/api/messages?with=${chatWithID}&offset=${chatOffset}`);
  chatLoading = false;
  if (!res.ok) return;
  const messages = await res.json();
  if (!messages || messages.length === 0) return;
 
  const container = document.getElementById('chat-messages');
  const prevHeight = container.scrollHeight;
 
  const html = messages.map(m => renderMsg(m)).join('');
  container.insertAdjacentHTML('afterbegin', html);
 
  chatOffset += messages.length;
 
  if (scrollToBottom) {
    container.scrollTop = container.scrollHeight;
  } else {
    container.scrollTop = container.scrollHeight - prevHeight;
  }
}
 
let scrollThrottle = false;
function onChatScroll() {
  const container = document.getElementById('chat-messages');
  if (container.scrollTop < 40 && !scrollThrottle) {
    scrollThrottle = true;
    loadMessages(false);
    setTimeout(() => { scrollThrottle = false; }, 1000);
  }
}
 
function renderMsg(m) {
  const isMine = m.sender_id === currentUser.id;
  return `<div class="msg-item ${isMine ? 'mine' : ''}">
    <span class="msg-nick">${escHtml(m.nickname)}</span>
    <span class="msg-time">${formatDate(m.created_at)}</span>
    <div class="msg-text">${escHtml(m.content)}</div>
  </div>`;
}
 
function sendMsg() {
  const input = document.getElementById('chat-input');
  const content = input.value.trim();
  if (!content || !chatWithID || !ws || ws.readyState !== WebSocket.OPEN) return;
  onStopTyping();
  ws.send(JSON.stringify({ type: 'chat', to_id: chatWithID, content }));
  input.value = '';
}
 
function connectWS() {
  const proto = location.protocol === 'https:' ? 'wss' : 'ws';
  ws = new WebSocket(`${proto}://${location.host}/ws`);
 
  ws.onopen = () => console.log('WS connected');
 
  ws.onmessage = (event) => {
    const msg = JSON.parse(event.data);
 
    if (msg.type === 'online') {
      onlineUsers.add(msg.user_id);
      refreshUsers();
    } else if (msg.type === 'offline') {
      onlineUsers.delete(msg.user_id);
      refreshUsers();
    } else if (msg.type === 'typing') {
      showTyping(msg.from_id, msg.from_nick);
    } else if (msg.type === 'stop_typing') {
      hideTyping(msg.from_id);
    } else if (msg.type === 'chat') {
      if (chatWithID && (msg.from_id === chatWithID || msg.to_id === chatWithID)) {
        const container = document.getElementById('chat-messages');
        const fakeMsg = {
          sender_id: msg.from_id,
          receiver_id: msg.to_id,
          nickname: msg.from_nick,
          content: msg.content,
          created_at: msg.created_at
        };
        container.insertAdjacentHTML('beforeend', renderMsg(fakeMsg));
        container.scrollTop = container.scrollHeight;
        chatOffset++;
      }
      refreshUsers();
    }
  };
 
  ws.onclose = () => {
    console.log('WS closed, reconnecting in 3s...');
    setTimeout(() => { if (currentUser) connectWS(); }, 3000);
  };
 
  ws.onerror = (err) => console.error('WS error', err);
}
 
function escHtml(str) {
  return String(str)
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/\n/g, '<br>');
}
function escAttr(str) {
  return String(str).replace(/'/g, "\\'");
}
function formatDate(str) {
  if (!str) return '';
  const clean = str.replace(' ', 'T');
  const d = new Date(clean);
  if (isNaN(d.getTime())) return str;
  return d.toLocaleDateString() + ' ' + d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
}
 
 
function onTyping() {
  if (!chatWithID || !ws || ws.readyState !== WebSocket.OPEN) return;
 
  if (!isTyping) {
    isTyping = true;
    ws.send(JSON.stringify({ type: 'typing', to_id: chatWithID }));
  }
 
  clearTimeout(typingTimer);
  typingTimer = setTimeout(() => {
    onStopTyping();
  }, 1500);
}
 
function onStopTyping() {
  if (!isTyping) return;
  isTyping = false;
  clearTimeout(typingTimer);
  if (chatWithID && ws && ws.readyState === WebSocket.OPEN) {
    ws.send(JSON.stringify({ type: 'stop_typing', to_id: chatWithID }));
  }
}
 
function showTyping(fromID, fromNick) {
  if (chatWithID !== fromID) return;
  document.getElementById('typing-name').textContent = fromNick + ' is typing';
  document.getElementById('typing-indicator').style.display = 'flex';
 
  clearTimeout(stopTypingTimers[fromID]);
  stopTypingTimers[fromID] = setTimeout(() => hideTyping(fromID), 3000);
}
 
function hideTyping(fromID) {
  if (chatWithID !== fromID) return;
  document.getElementById('typing-indicator').style.display = 'none';
  document.getElementById('typing-name').textContent = '';
}
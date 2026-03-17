// --- Chat ---

let chatAgentId = null;
let chatTemplateName = null;
let chatMessages = []; // {role, text, ts, thinking}
let chatBusy = false;
let chatAbortController = null;
let chatQueue = []; // queued messages typed while busy

function openChat(agentId, templateName) {
  chatAgentId = agentId;
  chatTemplateName = templateName;
  chatMessages = [];
  chatBusy = false;
  chatAbortController = null;
  chatQueue = [];
  const root = document.getElementById('chat-root');
  root.innerHTML = `
    <div class="chat-overlay" onclick="closeChat()"></div>
    <div class="chat-panel">
      <div class="chat-header">
        <h3>${esc(templateName)}</h3>
        <button class="btn btn-sm" onclick="closeChat()">Close</button>
      </div>
      <div class="chat-messages" id="chat-messages">
        <div style="text-align:center;color:var(--muted);font-size:12px;padding:20px">Send a message to ${esc(templateName)} agent</div>
      </div>
      <div class="chat-input-bar">
        <textarea id="chat-input" rows="1" placeholder="Type a message... (Enter to send, Shift+Enter for newline)"
          onkeydown="chatInputKeydown(event)" oninput="chatInputResize(this)"></textarea>
        <button id="chat-send" onclick="chatSendOrStop()">Send</button>
      </div>
      <div class="chat-queue-notice" id="chat-queue-notice" style="display:none"></div>
    </div>
  `;
  document.getElementById('chat-input').focus();
}

function closeChat() {
  if (chatAbortController) chatAbortController.abort();
  chatAgentId = null;
  chatMessages = [];
  chatBusy = false;
  chatAbortController = null;
  chatQueue = [];
  document.getElementById('chat-root').innerHTML = '';
}

function chatInputKeydown(e) {
  if (e.key === 'Enter' && !e.shiftKey) {
    e.preventDefault();
    chatSendOrStop();
  }
}

function chatInputResize(el) {
  el.style.height = 'auto';
  el.style.height = Math.min(el.scrollHeight, 120) + 'px';
}

function chatSendOrStop() {
  if (chatBusy) {
    // Stop current request
    if (chatAbortController) chatAbortController.abort();
    chatQueue = [];
    return;
  }
  const input = document.getElementById('chat-input');
  const message = input.value.trim();
  if (!message || !chatAgentId) return;
  input.value = '';
  input.style.height = 'auto';
  execChatMessage(message);
}

function chatEnqueue(message) {
  chatQueue.push(message);
  updateQueueNotice();
}

function updateQueueNotice() {
  const el = document.getElementById('chat-queue-notice');
  if (!el) return;
  if (chatQueue.length > 0) {
    el.style.display = 'block';
    el.textContent = chatQueue.length === 1
      ? `1 message queued — will send when agent finishes`
      : `${chatQueue.length} messages queued`;
  } else {
    el.style.display = 'none';
  }
}

function updateSendBtn() {
  const btn = document.getElementById('chat-send');
  if (!btn) return;
  if (chatBusy) {
    btn.textContent = 'Stop';
    btn.classList.add('stop');
  } else {
    btn.textContent = 'Send';
    btn.classList.remove('stop');
  }
}

function chatTimestamp() {
  const now = new Date();
  return now.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
}

async function execChatMessage(message) {
  chatBusy = true;
  updateSendBtn();

  const input = document.getElementById('chat-input');

  // Allow typing next message while busy — queue it on Enter
  if (input) {
    input.onkeydown = (e) => {
      if (e.key === 'Enter' && !e.shiftKey) {
        e.preventDefault();
        const queued = input.value.trim();
        if (queued) {
          chatEnqueue(queued);
          input.value = '';
          input.style.height = 'auto';
        }
      }
    };
  }

  chatMessages.push({ role: 'user', text: message, ts: chatTimestamp() });
  renderChatMessages();

  const agentMsgIdx = chatMessages.length;
  chatMessages.push({ role: 'agent', text: '', ts: chatTimestamp() });
  renderChatMessages();

  chatAbortController = new AbortController();

  try {
    const resp = await fetch('/agents/' + encodeURIComponent(chatAgentId) + '/chat', {
      method: 'POST',
      headers: { 'Authorization': 'Bearer ' + token, 'Content-Type': 'application/json' },
      body: JSON.stringify({ message, session: 'autobot-manager-default-chat' }),
      signal: chatAbortController.signal,
    });

    if (!resp.ok) {
      let errMsg = 'Error ' + resp.status;
      try { const j = await resp.json(); errMsg = j.error || errMsg; } catch(e) {}
      chatMessages[agentMsgIdx] = { role: 'error', text: errMsg, ts: chatTimestamp() };
      renderChatMessages();
    } else {
      const contentType = resp.headers.get('Content-Type') || '';
      if (contentType.includes('text/event-stream')) {
        const reader = resp.body.getReader();
        const decoder = new TextDecoder();
        let buffer = '';
        while (true) {
          const { done, value } = await reader.read();
          if (done) break;
          buffer += decoder.decode(value, { stream: true });
          const lines = buffer.split('\n');
          buffer = lines.pop();
          for (const line of lines) {
            if (line.startsWith('data: ')) {
              const payload = line.slice(6);
              try {
                const j = JSON.parse(payload);
                if (j.text || j.content || j.delta) {
                  chatMessages[agentMsgIdx].text += (j.text || j.content || j.delta);
                }
              } catch(e) {
                chatMessages[agentMsgIdx].text += payload;
              }
              renderChatMessages();
            }
          }
        }
        if (buffer.startsWith('data: ')) {
          chatMessages[agentMsgIdx].text += buffer.slice(6);
          renderChatMessages();
        }
      } else {
        const reader = resp.body.getReader();
        const decoder = new TextDecoder();
        while (true) {
          const { done, value } = await reader.read();
          if (done) break;
          chatMessages[agentMsgIdx].text += decoder.decode(value, { stream: true });
          renderChatMessages();
        }
        const raw = chatMessages[agentMsgIdx].text.trim();
        if (raw.startsWith('{')) {
          try {
            const j = JSON.parse(raw);
            const extracted = j.response || j.message || j.text || j.reply;
            chatMessages[agentMsgIdx].text = extracted || '';
            if (j.thinking) chatMessages[agentMsgIdx].thinking = j.thinking;
            if (j.error) chatMessages[agentMsgIdx] = { role: 'error', text: j.error, ts: chatTimestamp() };
          } catch(e) {}
        }
        renderChatMessages();
      }
    }
  } catch(e) {
    if (e.name === 'AbortError') {
      chatMessages[agentMsgIdx].text += chatMessages[agentMsgIdx].text ? ' _(stopped)_' : '_(stopped)_';
    } else {
      chatMessages[agentMsgIdx] = { role: 'error', text: 'Failed to reach agent: ' + e.message, ts: chatTimestamp() };
    }
    renderChatMessages();
  }

  chatBusy = false;
  chatAbortController = null;

  // Restore normal keydown
  if (input) {
    input.onkeydown = chatInputKeydown;
    input.focus();
  }

  updateSendBtn();
  updateQueueNotice();

  // Drain queue
  if (chatQueue.length > 0) {
    const next = chatQueue.shift();
    updateQueueNotice();
    execChatMessage(next);
  }
}

function renderChatMessages() {
  const el = document.getElementById('chat-messages');
  if (!el) return;
  el.innerHTML = chatMessages.map(m => {
    const ts = m.ts ? `<span class="chat-ts">${esc(m.ts)}</span>` : '';
    if (m.role === 'error') return `<div class="chat-msg error">${esc(m.text)}${ts}</div>`;
    if (m.role === 'agent') {
      const thinkingHtml = m.thinking
        ? `<details class="chat-thinking"><summary>Thinking...</summary><div class="chat-thinking-body">${esc(m.thinking)}</div></details>`
        : '';
      if (!m.text && !m.thinking) return `<div class="chat-msg agent"><span style="color:var(--muted);font-style:italic">...</span>${ts}</div>`;
      return `<div class="chat-msg agent">${thinkingHtml}${mdToHtml(m.text)}${ts}</div>`;
    }
    return `<div class="chat-msg ${m.role}">${esc(m.text)}${ts}</div>`;
  }).join('');
  el.scrollTop = el.scrollHeight;
}

// Lightweight markdown to HTML (handles bold, italic, code, lists, paragraphs)
function mdToHtml(md) {
  if (!md) return '';
  let html = esc(md);
  // Code blocks: ```...```
  html = html.replace(/```(\w*)\n([\s\S]*?)```/g, (_, lang, code) => `<pre><code>${code.trim()}</code></pre>`);
  // Inline code: `...`
  html = html.replace(/`([^`]+)`/g, '<code>$1</code>');
  // Bold: **...**
  html = html.replace(/\*\*(.+?)\*\*/g, '<strong>$1</strong>');
  // Italic: *...*
  html = html.replace(/(?<!\*)\*(?!\*)(.+?)(?<!\*)\*(?!\*)/g, '<em>$1</em>');
  // Headers: ### ... (at start of line)
  html = html.replace(/^### (.+)$/gm, '<h3>$1</h3>');
  html = html.replace(/^## (.+)$/gm, '<h2>$1</h2>');
  html = html.replace(/^# (.+)$/gm, '<h1>$1</h1>');
  // Unordered lists: * or - at start of line
  html = html.replace(/^[*-] (.+)$/gm, '<li>$1</li>');
  html = html.replace(/((?:<li>.*<\/li>\n?)+)/g, '<ul>$1</ul>');
  // Paragraphs: double newline
  html = html.replace(/\n\n+/g, '</p><p>');
  html = '<p>' + html + '</p>';
  // Clean up empty paragraphs and stray newlines inside paragraphs
  html = html.replace(/<p>\s*<\/p>/g, '');
  html = html.replace(/<p>\s*(<[hup])/g, '$1');
  html = html.replace(/(<\/[hup]l?>)\s*<\/p>/g, '$1');
  // Single newlines to <br> (but not inside pre/ul)
  html = html.replace(/(?<!<\/li|<\/pre|<\/ul|<\/ol|<\/h[123]>)\n/g, '<br>');
  return html;
}

/**
 * play_match.js - In-game player logic (layered on top of match.js spectator)
 */
(function() {
    'use strict';

    var boardEl = document.getElementById('game-board');
    var matchId = boardEl ? boardEl.dataset.matchId : '';
    var gameType = boardEl ? boardEl.dataset.gameType : '';
    var playerId = localStorage.getItem('claw_player_id') || '';
    var playerName = localStorage.getItem('claw_player_name') || '';

    if (!playerId) {
        window.location.href = '/play';
        return;
    }

    var ws = null;
    var reconnectDelay = 1000;
    var maxReconnectDelay = 30000;
    var isMyTurn = false;
    var myPlayerNum = 0; // 1 or 2
    var matchStatus = '';

    // Chat input
    var chatInput = document.getElementById('chat-input');
    var chatSendBtn = document.getElementById('chat-send-btn');

    if (chatSendBtn) {
        chatSendBtn.addEventListener('click', sendChat);
    }
    if (chatInput) {
        chatInput.addEventListener('keydown', function(e) {
            if (e.key === 'Enter') sendChat();
        });
    }

    function sendChat() {
        var msg = chatInput.value.trim();
        if (!msg) return;
        fetch('/api/match/' + matchId + '/chat', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ player_id: playerId, message: msg })
        }).then(function() {
            // Show own message in log
            if (window.MatchViewer) {
                window.MatchViewer.appendChat(playerName, msg, 'self');
            }
            chatInput.value = '';
        }).catch(function(err) {
            console.error('Chat failed:', err);
        });
    }

    // Expose player context to game-specific scripts
    window.PlayMatch = {
        matchId: matchId,
        gameType: gameType,
        playerId: playerId,
        playerName: playerName,
        myPlayerNum: 0,
        isMyTurn: false,
        matchStatus: '',
        ws: null,

        sendAction: function(actionType, actionData) {
            return fetch('/api/match/' + matchId + '/action', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({
                    player_id: playerId,
                    action_type: actionType,
                    action_data: actionData || {}
                })
            }).then(function(r) { return r.json(); });
        },

        sendReady: function() {
            return fetch('/api/match/' + matchId + '/ready', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ player_id: playerId })
            }).then(function(r) { return r.json(); });
        },

        getState: function() {
            return fetch('/api/match/' + matchId + '/state?player_id=' + playerId)
                .then(function(r) { return r.json(); });
        },

        refreshState: function() {
            return this.getState().then(function(state) {
                if (state && !state.error) {
                    handlePlayerState(state);
                }
                return state;
            });
        }
    };

    function handlePlayerState(state) {
        matchStatus = state.phase || state.status || '';
        window.PlayMatch.matchStatus = matchStatus;

        // The player state API returns your_turn as a boolean
        isMyTurn = !!state.your_turn;
        window.PlayMatch.isMyTurn = isMyTurn;

        // Dispatch custom event for game-specific renderers
        var evt = new CustomEvent('play_state_update', { detail: state });
        document.dispatchEvent(evt);
    }

    // Connect WebSocket for live events
    function connectWS() {
        if (!matchId) return;

        var proto = location.protocol === 'https:' ? 'wss:' : 'ws:';
        ws = new WebSocket(proto + '//' + location.host + '/ws');
        window.PlayMatch.ws = ws;

        ws.onopen = function() {
            reconnectDelay = 1000;
            // Register as player
            ws.send(JSON.stringify({
                type: 'register',
                player_id: playerId,
                player_name: playerName
            }));
        };

        ws.onmessage = function(evt) {
            try {
                var msg = JSON.parse(evt.data);
                handleWSMessage(msg);
            } catch (e) {
                console.error('Failed to parse WS message:', e);
            }
        };

        ws.onclose = function() {
            var wsStatusEl = document.getElementById('ws-status');
            if (wsStatusEl) {
                wsStatusEl.classList.remove('ws-connected');
                wsStatusEl.classList.add('ws-disconnected');
            }
            setTimeout(function() {
                reconnectDelay = Math.min(reconnectDelay * 2, maxReconnectDelay);
                connectWS();
            }, reconnectDelay);
        };

        ws.onerror = function() {
            ws.close();
        };
    }

    function handleWSMessage(msg) {
        var wsStatusEl = document.getElementById('ws-status');

        switch (msg.type) {
            case 'registered':
                if (wsStatusEl) {
                    wsStatusEl.classList.add('ws-connected');
                    wsStatusEl.classList.remove('ws-disconnected');
                }
                // Now fetch the player-specific state
                window.PlayMatch.refreshState();
                break;

            case 'your_turn':
            case 'opponent_action':
            case 'action_result':
            case 'prep_phase':
            case 'game_start':
            case 'game_state':
                // Refresh full player state on any game event
                window.PlayMatch.refreshState();
                break;

            case 'game_over':
                matchStatus = 'completed';
                window.PlayMatch.matchStatus = 'completed';
                window.PlayMatch.isMyTurn = false;
                if (window.MatchViewer) {
                    window.MatchViewer.showGameOver(msg.result || 'Match complete');
                }
                // Final state refresh
                window.PlayMatch.refreshState();
                break;

            case 'chat':
                if (window.MatchViewer) {
                    window.MatchViewer.appendChat(msg.from, msg.message, msg.scope);
                }
                break;

            case 'match_found':
            case 'match_joined':
                // Opponent joined, refresh state
                window.PlayMatch.refreshState();
                break;

            case 'opponent_left':
            case 'match_ended':
                matchStatus = 'waiting';
                window.PlayMatch.matchStatus = 'waiting';
                window.PlayMatch.isMyTurn = false;
                if (window.MatchViewer) {
                    window.MatchViewer.appendAction(null, msg.message || 'Opponent left the match');
                }
                window.PlayMatch.refreshState();
                break;

            case 'error':
                console.error('Server error:', msg.message);
                break;
        }

        // Forward to game-specific handlers
        var evt = new CustomEvent('play_ws_message', { detail: msg });
        document.dispatchEvent(evt);
    }

    connectWS();
})();

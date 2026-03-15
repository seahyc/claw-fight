/**
 * match.js - Core match viewer with WebSocket spectating
 */
(function() {
    'use strict';

    var boardEl = document.getElementById('game-board');
    var matchId = boardEl ? boardEl.dataset.matchId : '';
    var gameType = boardEl ? boardEl.dataset.gameType : '';
    var actionLog = document.getElementById('action-log');
    var statusEl = document.getElementById('match-status');
    var turnTextEl = document.getElementById('turn-indicator-text');
    var timerEl = document.getElementById('match-timer');
    var overlayEl = document.getElementById('game-over-overlay');
    var resultEl = document.getElementById('game-over-result');
    var p1NameEl = document.getElementById('p1-name');
    var p2NameEl = document.getElementById('p2-name');
    var p1EloEl = document.getElementById('p1-elo');
    var p2EloEl = document.getElementById('p2-elo');
    var turnP1El = document.getElementById('turn-p1');
    var turnP2El = document.getElementById('turn-p2');
    var wsStatusEl = document.getElementById('ws-status');

    var ws = null;
    var reconnectDelay = 1000;
    var maxReconnectDelay = 30000;
    var timerInterval = null;
    var turnDeadline = null;
    var lastMatchState = null;

    function setWsStatus(connected) {
        if (!wsStatusEl) return;
        wsStatusEl.classList.toggle('ws-connected', connected);
        wsStatusEl.classList.toggle('ws-disconnected', !connected);
        wsStatusEl.title = connected ? 'Connected' : 'Reconnecting...';
    }

    var MatchViewer = {
        matchId: matchId,
        gameType: gameType,
        state: null,
        players: { p1: null, p2: null },

        appendAction: function(player, text, timestamp) {
            var entry = document.createElement('div');
            entry.className = 'action-log-entry';

            var time = '';
            if (timestamp) {
                var d = new Date(timestamp);
                time = '<span class="action-log-time">' +
                    d.getHours().toString().padStart(2, '0') + ':' +
                    d.getMinutes().toString().padStart(2, '0') + ':' +
                    d.getSeconds().toString().padStart(2, '0') + '</span>';
            }

            var playerClass = '';
            if (player) {
                playerClass = player === MatchViewer.players.p1 ? 'p1' : 'p2';
            }

            entry.innerHTML = time +
                (player ? '<span class="action-log-player ' + playerClass + '">' + player + '</span> ' : '') +
                '<span class="action-log-text">' + text + '</span>';

            actionLog.appendChild(entry);
            actionLog.scrollTop = actionLog.scrollHeight;
        },

        setTurn: function(playerNum) {
            turnP1El.classList.toggle('active', playerNum === 1);
            turnP2El.classList.toggle('active', playerNum === 2);
            if (playerNum === 1) {
                turnTextEl.textContent = (MatchViewer.players.p1 || 'Player 1') + "'s turn";
            } else if (playerNum === 2) {
                turnTextEl.textContent = (MatchViewer.players.p2 || 'Player 2') + "'s turn";
            } else {
                turnTextEl.textContent = '';
            }
        },

        setStatus: function(status) {
            statusEl.textContent = status;
        },

        setTimer: function(deadline) {
            turnDeadline = deadline ? new Date(deadline).getTime() : null;
            if (timerInterval) clearInterval(timerInterval);
            if (!turnDeadline) {
                timerEl.textContent = '';
                timerEl.classList.remove('urgent');
                return;
            }
            timerInterval = setInterval(function() {
                var remaining = Math.max(0, Math.ceil((turnDeadline - Date.now()) / 1000));
                var mins = Math.floor(remaining / 60);
                var secs = remaining % 60;
                timerEl.textContent = mins + ':' + secs.toString().padStart(2, '0');
                timerEl.classList.toggle('urgent', remaining <= 10);
                if (remaining <= 0) clearInterval(timerInterval);
            }, 250);
        },

        showGameOver: function(result, isHistory) {
            if (resultEl) resultEl.textContent = result;
            if (overlayEl) {
                if (isHistory) {
                    // For historical matches, don't show the blocking overlay
                    overlayEl.classList.add('hidden');
                } else {
                    overlayEl.classList.remove('hidden');
                }
            }
            MatchViewer.setTurn(0);
            MatchViewer.setTimer(null);
            MatchViewer.setStatus('COMPLETE');
        },

        appendChat: function(from, message, scope) {
            var entry = document.createElement('div');
            entry.className = 'action-log-entry chat-entry';
            if (scope === 'opponent') entry.className += ' whisper';

            var playerClass = from === MatchViewer.players.p1 ? 'p1' : 'p2';
            entry.innerHTML =
                '<span class="chat-icon">\uD83D\uDCAC</span>' +
                '<span class="action-log-player ' + playerClass + '">' + from + '</span> ' +
                '<span class="chat-message">' + escapeHtml(message) + '</span>';

            actionLog.appendChild(entry);
            actionLog.scrollTop = actionLog.scrollHeight;
        },

        renderBoard: function(state) {
            MatchViewer.state = state;
            if (gameType === 'battleship' && typeof renderBattleshipBoard === 'function') {
                renderBattleshipBoard(state);
            } else if (gameType === 'poker' && typeof renderPokerBoard === 'function') {
                renderPokerBoard(state);
            } else if (gameType === 'prisoners_dilemma' && typeof renderPrisonersBoard === 'function') {
                renderPrisonersBoard(state);
            } else if (gameType === 'tictactoe' && typeof renderTictactoeBoard === 'function') {
                renderTictactoeBoard(state);
            }
        },

        restoreLastState: function() {
            if (!lastMatchState) return;
            handleMessage(lastMatchState);
        }
    };

    window.MatchViewer = MatchViewer;

    function escapeHtml(text) {
        var div = document.createElement('div');
        div.appendChild(document.createTextNode(text));
        return div.innerHTML;
    }

    function handleMessage(msg) {
        switch (msg.type) {
            case 'match_state':
                lastMatchState = msg;
                if (msg.players) {
                    MatchViewer.players.p1 = msg.players[0] ? msg.players[0].name : 'Player 1';
                    MatchViewer.players.p2 = msg.players[1] ? msg.players[1].name : 'Player 2';
                    p1NameEl.textContent = MatchViewer.players.p1;
                    p2NameEl.textContent = MatchViewer.players.p2;
                    if (msg.players[0] && msg.players[0].elo) p1EloEl.textContent = '(' + msg.players[0].elo + ')';
                    if (msg.players[1] && msg.players[1].elo) p2EloEl.textContent = '(' + msg.players[1].elo + ')';
                }
                if (msg.status) MatchViewer.setStatus(msg.status);
                if (msg.current_turn) MatchViewer.setTurn(msg.current_turn);
                if (msg.turn_deadline) MatchViewer.setTimer(msg.turn_deadline);
                if (msg.game_state) MatchViewer.renderBoard(msg.game_state);
                if (msg.action_log) {
                    actionLog.innerHTML = '';
                    msg.action_log.forEach(function(entry) {
                        MatchViewer.appendAction(entry.player, entry.text, entry.timestamp);
                    });
                }
                if (msg.status === 'completed' || msg.status === 'finished') {
                    MatchViewer.showGameOver(msg.result || 'Match finished');
                }
                break;

            case 'action':
                MatchViewer.appendAction(msg.player, msg.text, msg.timestamp);
                if (msg.game_state) MatchViewer.renderBoard(msg.game_state);
                if (msg.current_turn) MatchViewer.setTurn(msg.current_turn);
                if (msg.turn_deadline) MatchViewer.setTimer(msg.turn_deadline);
                break;

            case 'chat':
                MatchViewer.appendChat(msg.from, msg.message, msg.scope);
                break;

            case 'game_over':
                MatchViewer.showGameOver(msg.result || 'Match complete');
                if (msg.game_state) MatchViewer.renderBoard(msg.game_state);
                break;
        }
    }

    function connect() {
        if (!matchId) return;

        var proto = location.protocol === 'https:' ? 'wss:' : 'ws:';
        ws = new WebSocket(proto + '//' + location.host + '/ws/spectate/' + matchId);

        ws.onopen = function() {
            reconnectDelay = 1000;
            setWsStatus(true);
            MatchViewer.appendAction(null, 'Connected to match');
        };

        ws.onmessage = function(evt) {
            try {
                var msg = JSON.parse(evt.data);
                handleMessage(msg);
            } catch (e) {
                console.error('Failed to parse message:', e);
            }
        };

        ws.onclose = function() {
            setWsStatus(false);
            // Don't spam action log - just update the status indicator
            setTimeout(function() {
                reconnectDelay = Math.min(reconnectDelay * 2, maxReconnectDelay);
                connect();
            }, reconnectDelay);
        };

        ws.onerror = function() {
            ws.close();
        };
    }

    // Make the game-over overlay dismissible by clicking on it
    if (overlayEl) {
        overlayEl.addEventListener('click', function(e) {
            // Don't dismiss if clicking the "Back to Home" link
            if (e.target.tagName === 'A') return;
            overlayEl.classList.add('hidden');
        });
    }

    // Load match history for finished matches
    function loadMatchHistory() {
        if (!matchId) return;
        fetch('/api/match/' + matchId + '/history')
            .then(function(res) {
                if (!res.ok) return null;
                return res.json();
            })
            .then(function(data) {
                if (!data) return;
                if (data.status !== 'finished') return;

                // Set player info
                if (data.players && data.players.length > 0) {
                    MatchViewer.players.p1 = data.players[0] ? data.players[0].name : 'Player 1';
                    MatchViewer.players.p2 = data.players[1] ? data.players[1].name : 'Player 2';
                    p1NameEl.textContent = MatchViewer.players.p1;
                    p2NameEl.textContent = MatchViewer.players.p2;
                    if (data.players[0] && data.players[0].elo) p1EloEl.textContent = '(' + data.players[0].elo + ')';
                    if (data.players[1] && data.players[1].elo) p2EloEl.textContent = '(' + data.players[1].elo + ')';
                }

                // Populate the action log
                if (data.action_log && data.action_log.length > 0) {
                    actionLog.innerHTML = '';
                    data.action_log.forEach(function(entry) {
                        if (entry.action_type === 'chat') {
                            MatchViewer.appendChat(entry.player, entry.text);
                        } else {
                            MatchViewer.appendAction(entry.player, entry.text, entry.timestamp);
                        }
                    });
                }

                // Build list of events that have game_state snapshots for replay
                var replayEvents = [];
                if (data.action_log) {
                    data.action_log.forEach(function(entry, idx) {
                        if (entry.game_state) {
                            replayEvents.push({ index: idx, game_state: entry.game_state, entry: entry });
                        }
                    });
                }

                // If we have replayable events, set up the replay controls
                if (replayEvents.length > 0) {
                    initReplayControls(replayEvents, data.game_state);
                } else if (data.game_state) {
                    // No per-move snapshots; just render the final state
                    MatchViewer.renderBoard(data.game_state);
                }

                // Show result inline (not as blocking overlay)
                MatchViewer.showGameOver(data.result || 'Match finished', true);
                MatchViewer.appendAction(null, data.result || 'Match finished');
            })
            .catch(function(err) {
                console.error('Failed to load match history:', err);
            });
    }

    function initReplayControls(replayEvents, finalState) {
        var controlsEl = document.getElementById('replay-controls');
        var counterEl = document.getElementById('replay-counter');
        var btnFirst = document.getElementById('replay-first');
        var btnPrev = document.getElementById('replay-prev');
        var btnPlay = document.getElementById('replay-play');
        var btnNext = document.getElementById('replay-next');
        var btnLast = document.getElementById('replay-last');

        if (!controlsEl) return;
        controlsEl.classList.remove('hidden');

        var currentIdx = replayEvents.length - 1; // default to last move
        var playInterval = null;

        function highlightLogEntry(replayIdx) {
            var entries = actionLog.querySelectorAll('.action-log-entry');
            for (var i = 0; i < entries.length; i++) {
                entries[i].classList.remove('current');
            }
            if (replayIdx >= 0 && replayIdx < replayEvents.length) {
                var logIndex = replayEvents[replayIdx].index;
                if (entries[logIndex]) {
                    entries[logIndex].classList.add('current');
                    entries[logIndex].scrollIntoView({ block: 'nearest', behavior: 'smooth' });
                }
            }
        }

        function goTo(idx) {
            if (idx < 0) idx = 0;
            if (idx >= replayEvents.length) idx = replayEvents.length - 1;
            currentIdx = idx;
            counterEl.textContent = (currentIdx + 1) + ' / ' + replayEvents.length;
            MatchViewer.renderBoard(replayEvents[currentIdx].game_state);
            highlightLogEntry(currentIdx);
        }

        function stopAutoPlay() {
            if (playInterval) {
                clearInterval(playInterval);
                playInterval = null;
                btnPlay.classList.remove('active');
                btnPlay.innerHTML = '&#9654;';
            }
        }

        btnFirst.addEventListener('click', function() { stopAutoPlay(); goTo(0); });
        btnPrev.addEventListener('click', function() { stopAutoPlay(); goTo(currentIdx - 1); });
        btnNext.addEventListener('click', function() { stopAutoPlay(); goTo(currentIdx + 1); });
        btnLast.addEventListener('click', function() { stopAutoPlay(); goTo(replayEvents.length - 1); });

        btnPlay.addEventListener('click', function() {
            if (playInterval) {
                stopAutoPlay();
                return;
            }
            // If at the end, start from the beginning
            if (currentIdx >= replayEvents.length - 1) {
                goTo(0);
            }
            btnPlay.classList.add('active');
            btnPlay.innerHTML = '&#9646;&#9646;';
            playInterval = setInterval(function() {
                if (currentIdx >= replayEvents.length - 1) {
                    stopAutoPlay();
                    return;
                }
                goTo(currentIdx + 1);
            }, 500);
        });

        // Show the last move by default
        goTo(replayEvents.length - 1);
    }

    // Check if the match status (set by server template) indicates finished
    var initialStatus = statusEl ? statusEl.textContent.trim().toLowerCase() : '';
    if (initialStatus === 'finished' || initialStatus === 'completed' || initialStatus === 'ended') {
        loadMatchHistory();
    }

    connect();
})();

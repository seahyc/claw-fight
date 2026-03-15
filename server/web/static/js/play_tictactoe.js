/**
 * play_tictactoe.js - Interactive Tic Tac Toe UI for human players
 */
(function() {
    'use strict';

    var playerState = null;

    // Override spectator renderer
    var origRender = window.renderTictactoeBoard;
    window.renderTictactoeBoard = function(state) {
        if (window.PlayMatch && window.PlayMatch.playerId) {
            if (window.MatchViewer) window.MatchViewer.state = state;
            return;
        }
        if (origRender) origRender(state);
    };

    document.addEventListener('play_state_update', function(e) {
        var state = e.detail;
        if (!state || window.PlayMatch.gameType !== 'tictactoe') return;
        playerState = state;
        renderPlayerView(state);
    });

    function renderPlayerView(state) {
        var boardEl = document.getElementById('game-board');
        if (!boardEl) return;
        boardEl.innerHTML = '';

        if (state.status === 'waiting') {
            boardEl.innerHTML = '<div class="phase-banner">Waiting for opponent to join...</div>';
            return;
        }

        var gs = state.game_specific || state.game_state || {};
        var container = document.createElement('div');
        container.style.cssText = 'display:flex;flex-direction:column;align-items:center;gap:1rem;padding:1rem;';

        var p1Name = (window.MatchViewer && MatchViewer.players.p1) || 'Player 1';
        var p2Name = (window.MatchViewer && MatchViewer.players.p2) || 'Player 2';
        var myNum = window.PlayMatch.myPlayerNum;
        var myMark = myNum === 1 ? 'X' : 'O';
        var oppMark = myNum === 1 ? 'O' : 'X';
        var myName = myNum === 1 ? p1Name : p2Name;
        var oppName = myNum === 1 ? p2Name : p1Name;

        // Info bar
        var info = document.createElement('div');
        info.style.cssText = 'text-align:center;font-size:1.1rem;color:var(--text);';
        if (state.status === 'completed' || state.status === 'finished') {
            info.innerHTML = '<strong style="color:var(--gold);">Game Over</strong>';
        } else if (window.PlayMatch.isMyTurn) {
            info.innerHTML = 'Your turn — you are <strong style="color:' + (myMark === 'X' ? '#6aaeeb' : '#e86a7a') + '">' + myMark + '</strong>';
        } else {
            info.innerHTML = 'Waiting for <strong>' + oppName + '</strong>...';
        }
        container.appendChild(info);

        // Board
        // Board can be at state.board (PlayerView) or gs.board (spectator)
        var board = state.board || gs.board || [['','',''],['','',''],['','','']];
        var grid = document.createElement('div');
        grid.style.cssText = 'display:grid;grid-template-columns:repeat(3,100px);grid-template-rows:repeat(3,100px);gap:4px;background:var(--border-light);border-radius:var(--radius-md);overflow:hidden;';

        for (var r = 0; r < 3; r++) {
            for (var c = 0; c < 3; c++) {
                var cell = document.createElement('button');
                var pos = r * 3 + c;
                var mark = board[r][c];
                cell.style.cssText = 'display:flex;align-items:center;justify-content:center;background:var(--bg-card);border:none;font-size:2.5rem;font-weight:800;cursor:default;transition:all 0.15s;';

                if (mark === 'X') {
                    cell.textContent = 'X';
                    cell.style.color = '#6aaeeb';
                } else if (mark === 'O') {
                    cell.textContent = 'O';
                    cell.style.color = '#e86a7a';
                } else if (window.PlayMatch.isMyTurn && state.status !== 'completed' && state.status !== 'finished') {
                    cell.style.cursor = 'pointer';
                    cell.dataset.pos = pos;
                    cell.addEventListener('mouseenter', function() {
                        this.style.background = 'var(--bg-card-hover)';
                        this.textContent = myMark;
                        this.style.color = (myMark === 'X' ? '#6aaeeb' : '#e86a7a');
                        this.style.opacity = '0.4';
                    });
                    cell.addEventListener('mouseleave', function() {
                        this.style.background = 'var(--bg-card)';
                        this.textContent = '';
                        this.style.opacity = '1';
                    });
                    cell.addEventListener('click', function() {
                        sendMark(parseInt(this.dataset.pos));
                    });
                }

                grid.appendChild(cell);
            }
        }
        container.appendChild(grid);

        // Legend
        var legend = document.createElement('div');
        legend.style.cssText = 'display:flex;gap:1.5rem;font-size:0.9rem;color:var(--text-muted);';
        legend.innerHTML = '<span><strong style="color:#6aaeeb">X</strong> = ' + p1Name + '</span>' +
            '<span><strong style="color:#e86a7a">O</strong> = ' + p2Name + '</span>';
        container.appendChild(legend);

        boardEl.appendChild(container);
    }

    function sendMark(position) {
        var cells = document.querySelectorAll('#game-board button');
        cells.forEach(function(c) { c.disabled = true; c.style.cursor = 'default'; });

        window.PlayMatch.sendAction('mark', { position: position })
            .then(function(result) {
                if (!result.success) {
                    cells.forEach(function(c) { c.disabled = false; });
                }
            })
            .catch(function(err) {
                cells.forEach(function(c) { c.disabled = false; });
            });
    }
})();

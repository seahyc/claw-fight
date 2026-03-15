/**
 * play_tictactoe.js - Interactive Tic Tac Toe UI for human players
 */
(function() {
    'use strict';

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
        renderPlayerView(state);
    });

    function renderPlayerView(state) {
        var boardEl = document.getElementById('game-board');
        if (!boardEl) return;
        boardEl.innerHTML = '';

        if ((state.status === 'waiting' || state.phase === 'waiting')) {
            boardEl.innerHTML = '<div class="phase-banner">Waiting for opponent to join...</div>';
            return;
        }

        var gs = state.game_specific || {};
        var container = document.createElement('div');
        container.style.cssText = 'display:flex;flex-direction:column;align-items:center;gap:1rem;padding:1rem;';

        var p1Name = (window.MatchViewer && MatchViewer.players.p1) || 'Player 1';
        var p2Name = (window.MatchViewer && MatchViewer.players.p2) || 'Player 2';
        var myNum = window.PlayMatch.myPlayerNum;
        var myMark = myNum === 1 ? 'X' : 'O';
        var myName = myNum === 1 ? p1Name : p2Name;
        var oppName = myNum === 1 ? p2Name : p1Name;

        var board = state.board || [];
        var size = gs.board_size || board.length || 5;

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

        // Board grid
        var cellSize = size <= 3 ? 100 : (size <= 5 ? 70 : 50);
        var grid = document.createElement('div');
        grid.style.cssText = 'display:grid;grid-template-columns:repeat(' + size + ',' + cellSize + 'px);grid-template-rows:repeat(' + size + ',' + cellSize + 'px);gap:4px;background:var(--border-light);border-radius:var(--radius-md);overflow:hidden;';

        for (var r = 0; r < size; r++) {
            for (var c = 0; c < size; c++) {
                var cell = document.createElement('button');
                var pos = r * size + c;
                var mark = (board[r] && board[r][c]) || '';
                cell.style.cssText = 'display:flex;align-items:center;justify-content:center;background:var(--bg-card);border:none;font-size:' + (cellSize > 60 ? '2rem' : '1.5rem') + ';font-weight:800;cursor:default;transition:all 0.15s;';

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
            '<span><strong style="color:#e86a7a">O</strong> = ' + p2Name + '</span>' +
            '<span>' + (gs.win_length || 4) + ' in a row to win</span>';
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

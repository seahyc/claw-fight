/**
 * board_tictactoe.js - Tic Tac Toe renderer (dynamic board size)
 */
(function() {
    'use strict';

    function findWinningLine(board, winLen) {
        var size = board.length;
        var dirs = [[0,1],[1,0],[1,1],[1,-1]];
        for (var r = 0; r < size; r++) {
            for (var c = 0; c < size; c++) {
                var mark = board[r][c];
                if (!mark) continue;
                for (var d = 0; d < dirs.length; d++) {
                    var dr = dirs[d][0], dc = dirs[d][1];
                    var endR = r + dr * (winLen - 1), endC = c + dc * (winLen - 1);
                    if (endR < 0 || endR >= size || endC < 0 || endC >= size) continue;
                    var cells = [[r, c]];
                    var won = true;
                    for (var k = 1; k < winLen; k++) {
                        if (board[r + dr * k][c + dc * k] !== mark) { won = false; break; }
                        cells.push([r + dr * k, c + dc * k]);
                    }
                    if (won) return { mark: mark, cells: cells };
                }
            }
        }
        return null;
    }

    window.renderTictactoeBoard = function(state) {
        var boardEl = document.getElementById('game-board');
        if (!boardEl || !state) return;
        boardEl.innerHTML = '';

        var container = document.createElement('div');
        container.style.cssText = 'display:flex;flex-direction:column;align-items:center;gap:16px;padding:24px;';

        var board = state.board || [['','','','',''],['','','','',''],['','','','',''],['','','','',''],['','','','','']];
        var size = board.length;
        var winLen = state.win_length || 4;
        var totalCells = size * size;
        var win = findWinningLine(board, winLen);

        var winCells = {};
        if (win) {
            for (var w = 0; w < win.cells.length; w++) {
                winCells[win.cells[w][0] + ',' + win.cells[w][1]] = true;
            }
        }

        var p1Name = (window.MatchViewer && MatchViewer.players.p1) || 'Player 1';
        var p2Name = (window.MatchViewer && MatchViewer.players.p2) || 'Player 2';

        var turnInfo = document.createElement('div');
        turnInfo.style.cssText = 'font-size:1.1em;color:var(--text-muted,#aaa);margin-bottom:8px;';
        if (win) {
            var winnerName = win.mark === 'X' ? p1Name : p2Name;
            var winColor = win.mark === 'X' ? '#6aaeeb' : '#e86a7a';
            turnInfo.innerHTML = '<span style="color:' + winColor + ';font-weight:bold;">' + winnerName + ' (' + win.mark + ')</span> wins!';
        } else if (state.move_count >= totalCells) {
            turnInfo.textContent = 'Draw!';
            turnInfo.style.color = 'var(--gold)';
        } else {
            var currentName = state.current_player === 'X' ? p1Name : p2Name;
            var currentColor = state.current_player === 'X' ? '#6aaeeb' : '#e86a7a';
            turnInfo.innerHTML = '<span style="color:' + currentColor + '">' + currentName + '</span>\'s turn (' + state.current_player + ')';
        }
        container.appendChild(turnInfo);

        var cellSize = size <= 3 ? 100 : (size <= 5 ? 70 : 50);
        var grid = document.createElement('div');
        grid.style.cssText = 'display:grid;grid-template-columns:repeat(' + size + ',' + cellSize + 'px);grid-template-rows:repeat(' + size + ',' + cellSize + 'px);gap:4px;background:var(--border-light);border-radius:var(--radius-md);overflow:hidden;';

        for (var r = 0; r < size; r++) {
            for (var c = 0; c < size; c++) {
                var cell = document.createElement('div');
                var isWinCell = winCells[r + ',' + c];
                cell.style.cssText = 'display:flex;align-items:center;justify-content:center;' +
                    'background:' + (isWinCell ? 'rgba(232,184,74,0.2)' : 'var(--bg-card)') + ';' +
                    'font-size:' + (cellSize > 60 ? '2rem' : '1.5rem') + ';font-weight:800;cursor:default;' +
                    'transition:background 0.15s;' +
                    (isWinCell ? 'box-shadow:0 0 12px rgba(232,184,74,0.3);' : '');

                var mark = board[r][c];
                if (mark === 'X') {
                    cell.textContent = 'X';
                    cell.style.color = '#6aaeeb';
                } else if (mark === 'O') {
                    cell.textContent = 'O';
                    cell.style.color = '#e86a7a';
                }
                grid.appendChild(cell);
            }
        }
        container.appendChild(grid);

        var moveInfo = document.createElement('div');
        moveInfo.style.cssText = 'font-size:0.9em;color:var(--text-muted);';
        moveInfo.textContent = 'Moves: ' + (state.move_count || 0) + ' / ' + totalCells + '  |  ' + winLen + ' in a row to win';
        container.appendChild(moveInfo);

        var legend = document.createElement('div');
        legend.style.cssText = 'display:flex;gap:24px;font-size:0.95em;margin-top:8px;';
        legend.innerHTML = '<span style="color:#6aaeeb"><strong>X</strong> ' + p1Name + '</span>' +
            '<span style="color:#e86a7a"><strong>O</strong> ' + p2Name + '</span>';
        container.appendChild(legend);

        boardEl.appendChild(container);
    };
})();

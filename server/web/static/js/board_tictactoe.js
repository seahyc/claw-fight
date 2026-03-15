/**
 * board_tictactoe.js - Tic Tac Toe renderer
 */
(function() {
    'use strict';

    var winLines = [
        // Rows
        [[0,0],[0,1],[0,2]],
        [[1,0],[1,1],[1,2]],
        [[2,0],[2,1],[2,2]],
        // Columns
        [[0,0],[1,0],[2,0]],
        [[0,1],[1,1],[2,1]],
        [[0,2],[1,2],[2,2]],
        // Diagonals
        [[0,0],[1,1],[2,2]],
        [[0,2],[1,1],[2,0]]
    ];

    function findWinningLine(board) {
        for (var i = 0; i < winLines.length; i++) {
            var line = winLines[i];
            var a = board[line[0][0]][line[0][1]];
            var b = board[line[1][0]][line[1][1]];
            var c = board[line[2][0]][line[2][1]];
            if (a && a === b && b === c) {
                return { mark: a, cells: line };
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

        var board = state.board || [['','',''],['','',''],['','','']];
        var win = findWinningLine(board);

        // Build winning cell set for highlighting
        var winCells = {};
        if (win) {
            for (var w = 0; w < win.cells.length; w++) {
                winCells[win.cells[w][0] + ',' + win.cells[w][1]] = true;
            }
        }

        // Turn info
        var p1Name = (window.MatchViewer && MatchViewer.players.p1) || 'Player 1';
        var p2Name = (window.MatchViewer && MatchViewer.players.p2) || 'Player 2';

        var turnInfo = document.createElement('div');
        turnInfo.style.cssText = 'font-size:1.1em;color:var(--text-muted,#aaa);margin-bottom:8px;';
        if (win) {
            var winnerName = win.mark === 'X' ? p1Name : p2Name;
            var winColor = win.mark === 'X' ? '#4dabf7' : '#e94560';
            turnInfo.innerHTML = '<span style="color:' + winColor + ';font-weight:bold;">' + winnerName + ' (' + win.mark + ')</span> wins!';
        } else if (state.move_count >= 9) {
            turnInfo.textContent = 'Draw!';
            turnInfo.style.color = '#ffd43b';
        } else {
            var currentName = state.current_player === 'X' ? p1Name : p2Name;
            var currentColor = state.current_player === 'X' ? '#4dabf7' : '#e94560';
            turnInfo.innerHTML = '<span style="color:' + currentColor + '">' + currentName + '</span>\'s turn (' + state.current_player + ')';
        }
        container.appendChild(turnInfo);

        // Grid
        var grid = document.createElement('div');
        grid.style.cssText = 'display:grid;grid-template-columns:repeat(3,1fr);gap:6px;width:min(320px,80vw);aspect-ratio:1;';

        for (var r = 0; r < 3; r++) {
            for (var c = 0; c < 3; c++) {
                var cell = document.createElement('div');
                var isWinCell = winCells[r + ',' + c];
                cell.style.cssText = 'display:flex;align-items:center;justify-content:center;' +
                    'background:' + (isWinCell ? 'rgba(255,215,0,0.15)' : 'rgba(255,255,255,0.05)') + ';' +
                    'border-radius:8px;font-size:3em;font-weight:bold;cursor:default;' +
                    'transition:background 0.2s;aspect-ratio:1;' +
                    (isWinCell ? 'box-shadow:0 0 12px rgba(255,215,0,0.3);' : '');

                var mark = board[r][c];
                if (mark === 'X') {
                    cell.textContent = 'X';
                    cell.style.color = '#4dabf7';
                } else if (mark === 'O') {
                    cell.textContent = 'O';
                    cell.style.color = '#e94560';
                }

                grid.appendChild(cell);
            }
        }
        container.appendChild(grid);

        // Move count
        var moveInfo = document.createElement('div');
        moveInfo.style.cssText = 'font-size:0.9em;color:var(--text-muted,#888);';
        moveInfo.textContent = 'Moves: ' + (state.move_count || 0) + ' / 9';
        container.appendChild(moveInfo);

        // Player legend
        var legend = document.createElement('div');
        legend.style.cssText = 'display:flex;gap:24px;font-size:0.95em;margin-top:8px;';
        legend.innerHTML = '<span style="color:#4dabf7"><strong>X</strong> ' + p1Name + '</span>' +
            '<span style="color:#e94560"><strong>O</strong> ' + p2Name + '</span>';
        container.appendChild(legend);

        boardEl.appendChild(container);
    };
})();

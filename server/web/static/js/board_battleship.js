/**
 * board_battleship.js - Battleship board renderer
 */
(function() {
    'use strict';

    var COLS = ['A', 'B', 'C', 'D', 'E', 'F', 'G', 'H', 'I', 'J'];
    var ROWS = 10;
    var lastActionCell = null;

    function cellClass(cell) {
        if (!cell) return 'water';
        switch (cell.state || cell) {
            case 'hit': return 'hit';
            case 'miss': return 'miss';
            case 'sunk': return 'sunk';
            case 'ship': return 'ship';
            default: return 'water';
        }
    }

    function buildGrid(board, label, lastAction) {
        var wrapper = document.createElement('div');
        wrapper.className = 'battleship-board';

        var labelEl = document.createElement('div');
        labelEl.className = 'battleship-board-label';
        labelEl.textContent = label;
        wrapper.appendChild(labelEl);

        var grid = document.createElement('div');
        grid.className = 'battleship-grid';

        // Top-left corner (empty)
        var corner = document.createElement('div');
        corner.className = 'battleship-header';
        grid.appendChild(corner);

        // Column headers
        for (var c = 0; c < COLS.length; c++) {
            var colHeader = document.createElement('div');
            colHeader.className = 'battleship-header';
            colHeader.textContent = COLS[c];
            grid.appendChild(colHeader);
        }

        // Rows
        for (var r = 0; r < ROWS; r++) {
            var rowHeader = document.createElement('div');
            rowHeader.className = 'battleship-header';
            rowHeader.textContent = (r + 1).toString();
            grid.appendChild(rowHeader);

            for (var ci = 0; ci < COLS.length; ci++) {
                var cell = document.createElement('div');
                var key = COLS[ci] + (r + 1);
                var cellState = board && board[r] ? (board[r][ci] || board[r][key]) : null;
                cell.className = 'battleship-cell ' + cellClass(cellState);
                cell.dataset.coord = key;
                cell.title = key;

                if (lastAction && lastAction.col === ci && lastAction.row === r) {
                    cell.classList.add('last-action');
                }

                grid.appendChild(cell);
            }
        }

        wrapper.appendChild(grid);
        return wrapper;
    }

    function parseLastAction(state, playerIndex) {
        if (!state.last_action) return null;
        var la = state.last_action;
        if (la.target_player !== undefined && la.target_player !== playerIndex) return null;
        return { row: la.row, col: la.col };
    }

    window.renderBattleshipBoard = function(state) {
        var boardEl = document.getElementById('game-board');
        if (!boardEl || !state) return;

        boardEl.innerHTML = '';

        var p1Board = state.boards ? state.boards[0] : (state.player1_board || null);
        var p2Board = state.boards ? state.boards[1] : (state.player2_board || null);

        var p1Name = (window.MatchViewer && MatchViewer.players.p1) || 'Player 1';
        var p2Name = (window.MatchViewer && MatchViewer.players.p2) || 'Player 2';

        var la1 = parseLastAction(state, 0);
        var la2 = parseLastAction(state, 1);

        boardEl.appendChild(buildGrid(p1Board, p1Name, la1));
        boardEl.appendChild(buildGrid(p2Board, p2Name, la2));

        if (state.phase === 'setup') {
            var overlay = document.createElement('div');
            overlay.style.cssText = 'text-align:center;width:100%;padding:1rem;color:var(--text-muted);font-size:1rem;';
            overlay.textContent = 'Ship Placement Phase';
            boardEl.insertBefore(overlay, boardEl.firstChild);
        }
    };
})();

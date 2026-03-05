/**
 * board_battleship.js - Battleship board renderer for spectator view
 */
(function() {
    'use strict';

    var COLS = ['A', 'B', 'C', 'D', 'E', 'F', 'G', 'H', 'I', 'J'];
    var ROWS = 10;

    // Ship characters -> display info
    var CELL_MAP = {
        '.': { cls: 'water', sym: '', label: 'Water' },
        'C': { cls: 'ship ship-carrier', sym: '\u25A0', label: 'Carrier' },
        'B': { cls: 'ship ship-battleship', sym: '\u25A0', label: 'Battleship' },
        'R': { cls: 'ship ship-cruiser', sym: '\u25A0', label: 'Cruiser' },
        'S': { cls: 'ship ship-submarine', sym: '\u25A0', label: 'Submarine' },
        'D': { cls: 'ship ship-destroyer', sym: '\u25A0', label: 'Destroyer' },
        'H': { cls: 'hit', sym: '\u2716', label: 'Hit' },
        'M': { cls: 'miss', sym: '\u2022', label: 'Miss' },
        'X': { cls: 'hit', sym: '\u2716', label: 'Hit' },
        'O': { cls: 'miss', sym: '\u2022', label: 'Miss' }
    };

    function buildGrid(board, label) {
        var wrapper = document.createElement('div');
        wrapper.className = 'battleship-board';

        var labelEl = document.createElement('div');
        labelEl.className = 'battleship-board-label';
        labelEl.textContent = label;
        wrapper.appendChild(labelEl);

        var grid = document.createElement('div');
        grid.className = 'battleship-grid';

        // Top-left corner
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
                var val = '.';
                if (board && board[r] && board[r][ci] !== undefined) {
                    val = board[r][ci];
                }
                var info = CELL_MAP[val] || CELL_MAP['.'];
                cell.className = 'battleship-cell ' + info.cls;
                cell.dataset.coord = key;
                cell.title = key + ' - ' + info.label;
                if (info.sym) cell.textContent = info.sym;
                grid.appendChild(cell);
            }
        }

        wrapper.appendChild(grid);
        return wrapper;
    }

    // Ship legend
    function buildLegend() {
        var legend = document.createElement('div');
        legend.className = 'ship-legend';
        var ships = [
            { cls: 'ship-carrier', name: 'Carrier (5)', sym: '\u25A0' },
            { cls: 'ship-battleship', name: 'Battleship (4)', sym: '\u25A0' },
            { cls: 'ship-cruiser', name: 'Cruiser (3)', sym: '\u25A0' },
            { cls: 'ship-submarine', name: 'Submarine (3)', sym: '\u25A0' },
            { cls: 'ship-destroyer', name: 'Destroyer (2)', sym: '\u25A0' },
            { cls: 'hit', name: 'Hit', sym: '\u2716' },
            { cls: 'miss', name: 'Miss', sym: '\u2022' }
        ];
        ships.forEach(function(s) {
            var item = document.createElement('span');
            item.className = 'legend-item';
            var swatch = document.createElement('span');
            swatch.className = 'legend-swatch ' + s.cls;
            swatch.textContent = s.sym;
            item.appendChild(swatch);
            item.appendChild(document.createTextNode(' ' + s.name));
            legend.appendChild(item);
        });
        return legend;
    }

    window.renderBattleshipBoard = function(state) {
        var boardEl = document.getElementById('game-board');
        if (!boardEl || !state) return;

        boardEl.innerHTML = '';

        var players = window.MatchViewer ? window.MatchViewer.players : {};
        var p1Name = players.p1 || 'Player 1';
        var p2Name = players.p2 || 'Player 2';

        var p1Board = null;
        var p2Board = null;

        // state is player_views map: player_id -> PlayerView
        var keys = Object.keys(state);
        if (keys.length >= 2) {
            var v1 = state[keys[0]];
            var v2 = state[keys[1]];
            p1Board = v1 && v1.board ? v1.board.own : null;
            p2Board = v2 && v2.board ? v2.board.own : null;
        } else if (keys.length === 1) {
            var v = state[keys[0]];
            p1Board = v && v.board ? v.board.own : null;
        }

        if (state[keys[0]] && state[keys[0]].phase === 'setup') {
            var overlay = document.createElement('div');
            overlay.className = 'phase-banner';
            overlay.textContent = '\u2693 Ship Placement Phase \u2693';
            boardEl.appendChild(overlay);
        }

        boardEl.appendChild(buildLegend());

        var boardsRow = document.createElement('div');
        boardsRow.className = 'boards-row';
        boardsRow.appendChild(buildGrid(p1Board, p1Name));
        boardsRow.appendChild(buildGrid(p2Board, p2Name));
        boardEl.appendChild(boardsRow);
    };
})();

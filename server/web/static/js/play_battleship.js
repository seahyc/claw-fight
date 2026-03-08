/**
 * play_battleship.js - Interactive battleship board for human players
 */
(function() {
    'use strict';

    var COLS = ['A', 'B', 'C', 'D', 'E', 'F', 'G', 'H', 'I', 'J'];
    var ROWS = 10;
    var SHIPS = [
        { id: 'carrier', name: 'Carrier', size: 5 },
        { id: 'battleship', name: 'Battleship', size: 4 },
        { id: 'cruiser', name: 'Cruiser', size: 3 },
        { id: 'submarine', name: 'Submarine', size: 3 },
        { id: 'destroyer', name: 'Destroyer', size: 2 }
    ];

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

    var phase = 'waiting'; // waiting, setup, playing, completed
    var placedShips = {}; // ship_id -> { row, col, horizontal }
    var currentShipIdx = 0;
    var isHorizontal = true;
    var playerState = null;

    // Override the spectator board renderer when in play mode
    var origRender = window.renderBattleshipBoard;

    window.renderBattleshipBoard = function(state) {
        // In play mode, we handle rendering ourselves
        if (window.PlayMatch && window.PlayMatch.playerId) {
            // Still store state for reference
            if (window.MatchViewer) window.MatchViewer.state = state;
            return;
        }
        // Fallback to spectator renderer
        if (origRender) origRender(state);
    };

    document.addEventListener('play_state_update', function(e) {
        var state = e.detail;
        if (!state || window.PlayMatch.gameType !== 'battleship') return;
        playerState = state;
        renderPlayerView(state);
    });

    function renderPlayerView(state) {
        var boardEl = document.getElementById('game-board');
        if (!boardEl) return;
        boardEl.innerHTML = '';

        var gameState = state.game_state || {};
        var myView = null;

        // Try to find our player view
        if (gameState.views) {
            myView = gameState.views[window.PlayMatch.playerId];
        }
        if (!myView && gameState[window.PlayMatch.playerId]) {
            myView = gameState[window.PlayMatch.playerId];
        }

        var currentPhase = (myView && myView.phase) || gameState.phase || state.status || '';

        if (currentPhase === 'setup' || currentPhase === 'prep') {
            phase = 'setup';
            renderSetupPhase(boardEl, myView);
        } else if (currentPhase === 'playing' || currentPhase === 'active' || currentPhase === 'in_progress') {
            phase = 'playing';
            renderPlayPhase(boardEl, myView, state);
        } else if (currentPhase === 'completed' || currentPhase === 'finished') {
            phase = 'completed';
            // Let spectator renderer show final state
            if (origRender) origRender(gameState);
        } else if (state.status === 'waiting') {
            boardEl.innerHTML = '<div class="phase-banner">Waiting for opponent to join...</div>';
        }
    }

    function renderSetupPhase(boardEl, myView) {
        var container = document.createElement('div');
        container.className = 'play-battleship-setup';

        var banner = document.createElement('div');
        banner.className = 'phase-banner';
        banner.textContent = '\u2693 Place Your Ships \u2693';
        container.appendChild(banner);

        var instructions = document.createElement('div');
        instructions.className = 'setup-instructions';
        if (currentShipIdx < SHIPS.length) {
            var ship = SHIPS[currentShipIdx];
            instructions.innerHTML = 'Place: <strong>' + ship.name + '</strong> (size ' + ship.size + ') - ' +
                (isHorizontal ? 'Horizontal' : 'Vertical') +
                '<br><button id="rotate-btn" class="btn btn-small">Rotate (R)</button>';
        } else {
            instructions.innerHTML = 'All ships placed! <button id="ready-btn" class="btn btn-primary">Ready!</button>';
        }
        container.appendChild(instructions);

        // Build setup grid
        var grid = document.createElement('div');
        grid.className = 'battleship-grid';

        // Corner
        var corner = document.createElement('div');
        corner.className = 'battleship-header';
        grid.appendChild(corner);

        // Col headers
        for (var c = 0; c < COLS.length; c++) {
            var colH = document.createElement('div');
            colH.className = 'battleship-header';
            colH.textContent = COLS[c];
            grid.appendChild(colH);
        }

        // Build a board representation from placed ships
        var board = [];
        for (var r = 0; r < ROWS; r++) {
            board[r] = [];
            for (var ci = 0; ci < COLS.length; ci++) {
                board[r][ci] = '.';
            }
        }

        // Mark placed ships
        var shipKeys = Object.keys(placedShips);
        for (var si = 0; si < shipKeys.length; si++) {
            var shipId = shipKeys[si];
            var placement = placedShips[shipId];
            var shipDef = SHIPS.find(function(s) { return s.id === shipId; });
            if (!shipDef) continue;
            for (var s = 0; s < shipDef.size; s++) {
                var sr = placement.horizontal ? placement.row : placement.row + s;
                var sc = placement.horizontal ? placement.col + s : placement.col;
                if (sr < ROWS && sc < COLS.length) {
                    board[sr][sc] = shipId.charAt(0).toUpperCase();
                }
            }
        }

        // Rows
        for (var ri = 0; ri < ROWS; ri++) {
            var rowH = document.createElement('div');
            rowH.className = 'battleship-header';
            rowH.textContent = (ri + 1).toString();
            grid.appendChild(rowH);

            for (var cj = 0; cj < COLS.length; cj++) {
                var cell = document.createElement('div');
                var val = board[ri][cj];
                var info = CELL_MAP[val] || CELL_MAP['.'];
                cell.className = 'battleship-cell ' + info.cls + ' setup-cell';
                cell.dataset.row = ri;
                cell.dataset.col = cj;
                if (info.sym) cell.textContent = info.sym;

                if (currentShipIdx < SHIPS.length) {
                    cell.addEventListener('click', handleSetupClick);
                    cell.addEventListener('mouseenter', handleSetupHover);
                    cell.addEventListener('mouseleave', handleSetupUnhover);
                }

                grid.appendChild(cell);
            }
        }

        container.appendChild(grid);
        boardEl.appendChild(container);

        // Bind rotate button
        var rotateBtn = document.getElementById('rotate-btn');
        if (rotateBtn) {
            rotateBtn.addEventListener('click', function() {
                isHorizontal = !isHorizontal;
                renderPlayerView(playerState);
            });
        }

        // Bind ready button
        var readyBtn = document.getElementById('ready-btn');
        if (readyBtn) {
            readyBtn.addEventListener('click', function() {
                submitShipPlacements();
            });
        }

        // Keyboard rotation
        document.onkeydown = function(e) {
            if (e.key === 'r' || e.key === 'R') {
                isHorizontal = !isHorizontal;
                renderPlayerView(playerState);
            }
        };
    }

    function handleSetupHover(e) {
        if (currentShipIdx >= SHIPS.length) return;
        var row = parseInt(e.target.dataset.row);
        var col = parseInt(e.target.dataset.col);
        var ship = SHIPS[currentShipIdx];
        var cells = getShipCells(row, col, ship.size, isHorizontal);
        var valid = isValidPlacement(cells);

        cells.forEach(function(c) {
            var cell = document.querySelector('.setup-cell[data-row="' + c.r + '"][data-col="' + c.c + '"]');
            if (cell) {
                cell.classList.add(valid ? 'hover-valid' : 'hover-invalid');
            }
        });
    }

    function handleSetupUnhover() {
        document.querySelectorAll('.hover-valid, .hover-invalid').forEach(function(c) {
            c.classList.remove('hover-valid', 'hover-invalid');
        });
    }

    function handleSetupClick(e) {
        if (currentShipIdx >= SHIPS.length) return;
        var row = parseInt(e.target.dataset.row);
        var col = parseInt(e.target.dataset.col);
        var ship = SHIPS[currentShipIdx];
        var cells = getShipCells(row, col, ship.size, isHorizontal);

        if (!isValidPlacement(cells)) return;

        placedShips[ship.id] = { row: row, col: col, horizontal: isHorizontal };
        currentShipIdx++;
        renderPlayerView(playerState);
    }

    function getShipCells(row, col, size, horizontal) {
        var cells = [];
        for (var i = 0; i < size; i++) {
            cells.push({
                r: horizontal ? row : row + i,
                c: horizontal ? col + i : col
            });
        }
        return cells;
    }

    function isValidPlacement(cells) {
        // Check bounds
        for (var i = 0; i < cells.length; i++) {
            if (cells[i].r >= ROWS || cells[i].c >= COLS.length || cells[i].r < 0 || cells[i].c < 0) {
                return false;
            }
        }

        // Check overlap with placed ships
        var occupied = {};
        var shipKeys = Object.keys(placedShips);
        for (var si = 0; si < shipKeys.length; si++) {
            var shipId = shipKeys[si];
            var placement = placedShips[shipId];
            var shipDef = SHIPS.find(function(s) { return s.id === shipId; });
            if (!shipDef) continue;
            for (var s = 0; s < shipDef.size; s++) {
                var r = placement.horizontal ? placement.row : placement.row + s;
                var c = placement.horizontal ? placement.col + s : placement.col;
                occupied[r + ',' + c] = true;
            }
        }

        for (var j = 0; j < cells.length; j++) {
            if (occupied[cells[j].r + ',' + cells[j].c]) return false;
        }

        return true;
    }

    function submitShipPlacements() {
        var placements = {};
        SHIPS.forEach(function(ship) {
            var p = placedShips[ship.id];
            if (p) {
                placements[ship.id] = {
                    position: COLS[p.col] + (p.row + 1),
                    orientation: p.horizontal ? 'horizontal' : 'vertical'
                };
            }
        });

        window.PlayMatch.sendAction('place_ships', { ships: placements })
            .then(function(result) {
                if (result.success) {
                    window.PlayMatch.sendReady().then(function() {
                        window.PlayMatch.refreshState();
                    });
                } else {
                    alert('Placement failed: ' + (result.message || 'Unknown error'));
                }
            })
            .catch(function(err) {
                alert('Error: ' + err.message);
            });
    }

    function renderPlayPhase(boardEl, myView, state) {
        var container = document.createElement('div');
        container.className = 'play-battleship-game';

        var myBoard = myView && myView.board ? myView.board.own : null;
        var enemyBoard = myView && myView.board ? myView.board.opponent : null;

        var isMyTurn = window.PlayMatch.isMyTurn;

        // Turn banner
        var turnBanner = document.createElement('div');
        turnBanner.className = 'phase-banner ' + (isMyTurn ? 'your-turn' : 'wait-turn');
        turnBanner.textContent = isMyTurn ? 'YOUR TURN - Fire!' : "Opponent's turn...";
        container.appendChild(turnBanner);

        var boardsRow = document.createElement('div');
        boardsRow.className = 'boards-row';

        // Own board (no interaction)
        boardsRow.appendChild(buildGrid(myBoard, 'Your Fleet', null, null, false));

        // Enemy board (clickable when it's our turn)
        boardsRow.appendChild(buildGrid(enemyBoard, 'Enemy Waters', state.game_state ? state.game_state.last_action : null, null, isMyTurn));

        container.appendChild(boardsRow);
        boardEl.appendChild(container);
    }

    function buildGrid(board, label, lastAction, sunkInfo, clickable) {
        var wrapper = document.createElement('div');
        wrapper.className = 'battleship-board';

        var labelEl = document.createElement('div');
        labelEl.className = 'battleship-board-label';
        labelEl.textContent = label;
        wrapper.appendChild(labelEl);

        var grid = document.createElement('div');
        grid.className = 'battleship-grid';

        var corner = document.createElement('div');
        corner.className = 'battleship-header';
        grid.appendChild(corner);

        for (var c = 0; c < COLS.length; c++) {
            var colH = document.createElement('div');
            colH.className = 'battleship-header';
            colH.textContent = COLS[c];
            grid.appendChild(colH);
        }

        for (var r = 0; r < ROWS; r++) {
            var rowH = document.createElement('div');
            rowH.className = 'battleship-header';
            rowH.textContent = (r + 1).toString();
            grid.appendChild(rowH);

            for (var ci = 0; ci < COLS.length; ci++) {
                var cell = document.createElement('div');
                var key = COLS[ci] + (r + 1);
                var val = '.';
                if (board && board[r] && board[r][ci] !== undefined) {
                    val = board[r][ci];
                }
                var info = CELL_MAP[val] || CELL_MAP['.'];
                cell.className = 'battleship-cell ' + info.cls;

                if (lastAction && lastAction.target === key) {
                    cell.className += ' last-shot';
                }

                if (clickable && (val === '.' || val === undefined)) {
                    cell.className += ' clickable';
                    cell.dataset.target = key;
                    cell.addEventListener('click', handleFireClick);
                }

                cell.title = key + ' - ' + info.label;
                if (info.sym) cell.textContent = info.sym;
                grid.appendChild(cell);
            }
        }

        wrapper.appendChild(grid);
        return wrapper;
    }

    function handleFireClick(e) {
        if (!window.PlayMatch.isMyTurn) return;
        var target = e.target.dataset.target;
        if (!target) return;

        // Disable further clicks
        document.querySelectorAll('.clickable').forEach(function(c) {
            c.classList.remove('clickable');
            c.removeEventListener('click', handleFireClick);
        });

        e.target.classList.add('firing');

        window.PlayMatch.sendAction('fire', { target: target })
            .then(function(result) {
                if (!result.success) {
                    alert('Fire failed: ' + (result.message || 'Unknown error'));
                }
                // State will refresh via WS event
            })
            .catch(function(err) {
                alert('Error: ' + err.message);
            });
    }
})();

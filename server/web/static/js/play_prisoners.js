/**
 * play_prisoners.js - Interactive Prisoner's Dilemma UI for human players
 */
(function() {
    'use strict';

    var playerState = null;

    // Override spectator renderer
    var origRender = window.renderPrisonersBoard;
    window.renderPrisonersBoard = function(state) {
        if (window.PlayMatch && window.PlayMatch.playerId) {
            if (window.MatchViewer) window.MatchViewer.state = state;
            return;
        }
        if (origRender) origRender(state);
    };

    document.addEventListener('play_state_update', function(e) {
        var state = e.detail;
        if (!state || window.PlayMatch.gameType !== 'prisoners_dilemma') return;
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

        var gameState = state.game_state || {};
        var container = document.createElement('div');
        container.className = 'prisoners-container';

        var p1Name = (window.MatchViewer && MatchViewer.players.p1) || 'Player 1';
        var p2Name = (window.MatchViewer && MatchViewer.players.p2) || 'Player 2';
        var myNum = window.PlayMatch.myPlayerNum;
        var myName = myNum === 1 ? p1Name : p2Name;
        var oppName = myNum === 1 ? p2Name : p1Name;

        // Round info
        var roundInfo = document.createElement('div');
        roundInfo.className = 'prisoners-round-info';
        roundInfo.innerHTML = 'Round <strong>' + (gameState.current_round || 0) + '</strong> / ' + (gameState.total_rounds || '?');
        container.appendChild(roundInfo);

        // Chaos event banner
        if (gameState.current_event) {
            var eventBanner = document.createElement('div');
            eventBanner.className = 'prisoners-event-banner';
            eventBanner.textContent = gameState.current_event.description || gameState.current_event.type || 'CHAOS EVENT';
            container.appendChild(eventBanner);
        }

        // Scores
        var scores = document.createElement('div');
        scores.className = 'prisoners-scores';

        var myIdx = myNum - 1;
        var oppIdx = myNum === 1 ? 1 : 0;

        var myScore = document.createElement('div');
        myScore.className = 'prisoners-player-score';
        var myCoopRate = gameState.cooperation_rates ? gameState.cooperation_rates[myIdx] : null;
        myScore.innerHTML = '<div class="score-name" style="color:#4dabf7">' + myName + ' (You)</div>' +
            '<div class="score-value">' + (gameState.scores ? gameState.scores[myIdx] : 0) + '</div>' +
            (myCoopRate !== null ? '<div class="coop-rate">Cooperation: ' + Math.round(myCoopRate * 100) + '%</div>' : '');
        scores.appendChild(myScore);

        var oppScore = document.createElement('div');
        oppScore.className = 'prisoners-player-score';
        var oppCoopRate = gameState.cooperation_rates ? gameState.cooperation_rates[oppIdx] : null;
        oppScore.innerHTML = '<div class="score-name" style="color:#e94560">' + oppName + '</div>' +
            '<div class="score-value">' + (gameState.scores ? gameState.scores[oppIdx] : 0) + '</div>' +
            (oppCoopRate !== null ? '<div class="coop-rate">Cooperation: ' + Math.round(oppCoopRate * 100) + '%</div>' : '');
        scores.appendChild(oppScore);

        container.appendChild(scores);

        // Action buttons (only when it's our turn)
        if (window.PlayMatch.isMyTurn && state.status !== 'completed') {
            var actions = document.createElement('div');
            actions.className = 'prisoners-actions';

            var coopBtn = document.createElement('button');
            coopBtn.className = 'btn prisoners-btn cooperate-btn';
            coopBtn.innerHTML = '<span class="prisoners-btn-icon">\u{1F91D}</span><span class="prisoners-btn-label">COOPERATE</span>';
            coopBtn.addEventListener('click', function() { sendChoice('cooperate'); });
            actions.appendChild(coopBtn);

            var defectBtn = document.createElement('button');
            defectBtn.className = 'btn prisoners-btn defect-btn';
            defectBtn.innerHTML = '<span class="prisoners-btn-icon">\u{1F5E1}</span><span class="prisoners-btn-label">DEFECT</span>';
            defectBtn.addEventListener('click', function() { sendChoice('defect'); });
            actions.appendChild(defectBtn);

            container.appendChild(actions);
        } else if (!window.PlayMatch.isMyTurn && state.status !== 'completed' && state.status !== 'waiting') {
            var waitBanner = document.createElement('div');
            waitBanner.className = 'phase-banner wait-turn';
            waitBanner.textContent = 'Waiting for opponent...';
            container.appendChild(waitBanner);
        }

        // Move history
        var moves = gameState.moves || gameState.history || [];
        if (moves.length > 0) {
            var historySection = document.createElement('div');
            historySection.className = 'prisoners-history-section';

            var histTitle = document.createElement('div');
            histTitle.className = 'prisoners-moves-label';
            histTitle.textContent = 'Round History';
            historySection.appendChild(histTitle);

            var histTable = document.createElement('div');
            histTable.className = 'prisoners-history-table';

            moves.forEach(function(m, idx) {
                var row = document.createElement('div');
                row.className = 'prisoners-history-row';

                var roundNum = document.createElement('span');
                roundNum.className = 'round-num';
                roundNum.textContent = 'R' + (idx + 1);
                row.appendChild(roundNum);

                var myMove = m[myIdx] || m['p' + myNum];
                var oppMove = m[oppIdx] || m['p' + (myNum === 1 ? 2 : 1)];

                var myMoveEl = document.createElement('span');
                var myIsCooperate = myMove === 'C' || myMove === 'cooperate';
                myMoveEl.className = 'prisoners-move ' + (myIsCooperate ? 'cooperate' : 'defect');
                myMoveEl.textContent = myIsCooperate ? 'C' : 'D';
                row.appendChild(myMoveEl);

                var oppMoveEl = document.createElement('span');
                var oppIsCooperate = oppMove === 'C' || oppMove === 'cooperate';
                oppMoveEl.className = 'prisoners-move ' + (oppIsCooperate ? 'cooperate' : 'defect');
                oppMoveEl.textContent = oppIsCooperate ? 'C' : 'D';
                row.appendChild(oppMoveEl);

                histTable.appendChild(row);
            });

            historySection.appendChild(histTable);
            container.appendChild(historySection);
        }

        boardEl.appendChild(container);
    }

    function sendChoice(choice) {
        var btns = document.querySelectorAll('.prisoners-btn');
        btns.forEach(function(b) { b.disabled = true; });

        window.PlayMatch.sendAction('choose', { choice: choice })
            .then(function(result) {
                if (!result.success) {
                    alert('Action failed: ' + (result.message || 'Unknown error'));
                    btns.forEach(function(b) { b.disabled = false; });
                }
            })
            .catch(function(err) {
                alert('Error: ' + err.message);
                btns.forEach(function(b) { b.disabled = false; });
            });
    }
})();

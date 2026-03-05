/**
 * board_prisoners.js - Prisoner's Dilemma renderer
 */
(function() {
    'use strict';

    var chartCanvas = null;

    function drawChart(canvas, scoreHistory) {
        if (!canvas || !scoreHistory || scoreHistory.length === 0) return;

        var ctx = canvas.getContext('2d');
        var w = canvas.width = canvas.parentElement.clientWidth - 32;
        var h = canvas.height = canvas.parentElement.clientHeight - 32;

        ctx.clearRect(0, 0, w, h);

        var p1Scores = scoreHistory.map(function(s) { return s[0]; });
        var p2Scores = scoreHistory.map(function(s) { return s[1]; });
        var maxScore = Math.max(Math.max.apply(null, p1Scores), Math.max.apply(null, p2Scores), 1);
        var len = scoreHistory.length;

        // Grid lines
        ctx.strokeStyle = 'rgba(255,255,255,0.05)';
        ctx.lineWidth = 1;
        for (var g = 0; g < 5; g++) {
            var gy = h - (g / 4) * h;
            ctx.beginPath();
            ctx.moveTo(0, gy);
            ctx.lineTo(w, gy);
            ctx.stroke();
        }

        function drawLine(scores, color) {
            ctx.strokeStyle = color;
            ctx.lineWidth = 2;
            ctx.beginPath();
            for (var i = 0; i < len; i++) {
                var x = (i / Math.max(len - 1, 1)) * w;
                var y = h - (scores[i] / maxScore) * h;
                if (i === 0) ctx.moveTo(x, y);
                else ctx.lineTo(x, y);
            }
            ctx.stroke();
        }

        drawLine(p1Scores, '#4dabf7');
        drawLine(p2Scores, '#e94560');
    }

    window.renderPrisonersBoard = function(state) {
        var boardEl = document.getElementById('game-board');
        if (!boardEl || !state) return;

        boardEl.innerHTML = '';

        var container = document.createElement('div');
        container.className = 'prisoners-container';

        var p1Name = (window.MatchViewer && MatchViewer.players.p1) || 'Player 1';
        var p2Name = (window.MatchViewer && MatchViewer.players.p2) || 'Player 2';

        // Round info
        var roundInfo = document.createElement('div');
        roundInfo.className = 'prisoners-round-info';
        roundInfo.innerHTML = 'Round <strong>' + (state.current_round || 0) + '</strong> / ' + (state.total_rounds || '?');
        container.appendChild(roundInfo);

        // Scores
        var scores = document.createElement('div');
        scores.className = 'prisoners-scores';

        var p1Score = document.createElement('div');
        p1Score.className = 'prisoners-player-score';
        var p1CoopRate = state.cooperation_rates ? state.cooperation_rates[0] : null;
        p1Score.innerHTML = '<div class="score-name" style="color:#4dabf7">' + p1Name + '</div>' +
            '<div class="score-value">' + (state.scores ? state.scores[0] : 0) + '</div>' +
            (p1CoopRate !== null ? '<div class="coop-rate">Cooperation: ' + Math.round(p1CoopRate * 100) + '%</div>' : '');
        scores.appendChild(p1Score);

        var p2Score = document.createElement('div');
        p2Score.className = 'prisoners-player-score';
        var p2CoopRate = state.cooperation_rates ? state.cooperation_rates[1] : null;
        p2Score.innerHTML = '<div class="score-name" style="color:#e94560">' + p2Name + '</div>' +
            '<div class="score-value">' + (state.scores ? state.scores[1] : 0) + '</div>' +
            (p2CoopRate !== null ? '<div class="coop-rate">Cooperation: ' + Math.round(p2CoopRate * 100) + '%</div>' : '');
        scores.appendChild(p2Score);

        container.appendChild(scores);

        // Score chart
        if (state.score_history && state.score_history.length > 1) {
            var chartContainer = document.createElement('div');
            chartContainer.className = 'prisoners-chart-container';
            var canvas = document.createElement('canvas');
            chartContainer.appendChild(canvas);
            container.appendChild(chartContainer);
            // Defer drawing to let container size settle
            setTimeout(function() { drawChart(canvas, state.score_history); }, 0);
        }

        // Move history
        var moves = state.moves || state.history || [];
        if (moves.length > 0) {
            var label1 = document.createElement('div');
            label1.className = 'prisoners-moves-label';
            label1.textContent = p1Name + ' moves';
            container.appendChild(label1);

            var history1 = document.createElement('div');
            history1.className = 'prisoners-history';
            moves.forEach(function(m) {
                var move = m[0] || m.p1;
                var moveEl = document.createElement('div');
                var isCooperate = move === 'C' || move === 'cooperate';
                moveEl.className = 'prisoners-move ' + (isCooperate ? 'cooperate' : 'defect');
                moveEl.textContent = isCooperate ? 'C' : 'D';
                history1.appendChild(moveEl);
            });
            container.appendChild(history1);

            var label2 = document.createElement('div');
            label2.className = 'prisoners-moves-label';
            label2.textContent = p2Name + ' moves';
            container.appendChild(label2);

            var history2 = document.createElement('div');
            history2.className = 'prisoners-history';
            moves.forEach(function(m) {
                var move = m[1] || m.p2;
                var moveEl = document.createElement('div');
                var isCooperate = move === 'C' || move === 'cooperate';
                moveEl.className = 'prisoners-move ' + (isCooperate ? 'cooperate' : 'defect');
                moveEl.textContent = isCooperate ? 'C' : 'D';
                history2.appendChild(moveEl);
            });
            container.appendChild(history2);
        }

        boardEl.appendChild(container);
    };
})();

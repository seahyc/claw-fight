/**
 * tournament.js - Tournament detail page
 */
(function() {
    'use strict';

    var tournamentId = TournamentPage.id;
    var statusEl = document.getElementById('tournament-status');
    var standingsEl = document.getElementById('standings-table');
    var roundsEl = document.getElementById('rounds-list');
    var actionsEl = document.getElementById('tournament-actions');
    var registerBtn = document.getElementById('btn-register');
    var refreshTimer = null;

    function fetchTournament() {
        fetch('/api/tournament/' + tournamentId)
            .then(function(r) { return r.json(); })
            .then(function(data) {
                renderTournament(data);
            })
            .catch(function() {
                standingsEl.innerHTML = '<p class="muted">Failed to load tournament data</p>';
            });
    }

    function renderTournament(data) {
        var t = data.tournament;
        var standings = data.standings || [];

        // Status badge
        var statusClass = '';
        if (t.status === 'open') statusClass = 'status-open';
        else if (t.status === 'active') statusClass = 'status-active';
        else if (t.status === 'finished') statusClass = 'status-finished';
        statusEl.className = 'badge ' + statusClass;
        statusEl.textContent = t.status;

        // Show register button if open
        if (t.status === 'open') {
            actionsEl.style.display = '';
            registerBtn.style.display = '';
        } else {
            actionsEl.style.display = 'none';
        }

        renderStandings(standings);
        renderRounds(t.rounds || []);

        // Auto-refresh during active tournaments
        if (refreshTimer) clearInterval(refreshTimer);
        if (t.status === 'active') {
            refreshTimer = setInterval(fetchTournament, 15000);
        }
    }

    function renderStandings(standings) {
        if (!standings || standings.length === 0) {
            standingsEl.innerHTML = '<p class="muted">No players registered yet</p>';
            return;
        }

        var html = '<table class="leaderboard-table">' +
            '<thead><tr>' +
            '<th>Rank</th><th>Player</th><th>W</th><th>L</th><th>D</th><th>Points</th>' +
            '</tr></thead><tbody>';

        standings.forEach(function(e, i) {
            html += '<tr>' +
                '<td class="rank">' + (i + 1) + '</td>' +
                '<td><a href="/player/' + e.player_id + '">' + e.player_id + '</a></td>' +
                '<td>' + e.wins + '</td>' +
                '<td>' + e.losses + '</td>' +
                '<td>' + e.draws + '</td>' +
                '<td class="elo">' + e.points.toFixed(1) + '</td>' +
                '</tr>';
        });

        html += '</tbody></table>';
        standingsEl.innerHTML = html;
    }

    function renderRounds(rounds) {
        if (!rounds || rounds.length === 0) {
            roundsEl.innerHTML = '<p class="muted">No rounds played yet</p>';
            return;
        }

        var html = '';
        rounds.forEach(function(round) {
            html += '<div class="round-section">' +
                '<h4 class="round-header" data-round="' + round.number + '">Round ' + round.number + '</h4>' +
                '<div class="round-matches">';

            round.matches.forEach(function(m) {
                var statusClass = '';
                if (m.status === 'finished') statusClass = 'status-finished';
                else if (m.status === 'active') statusClass = 'status-active';
                else statusClass = 'status-pending';

                var matchLink = m.player2 === 'BYE' ? '#' : '/match/' + m.match_id;
                var p1Class = m.winner === m.player1 ? 'winner' : '';
                var p2Class = m.winner === m.player2 ? 'winner' : '';

                html += '<a href="' + matchLink + '" class="match-card round-match">' +
                    '<div class="match-card-players">' +
                        '<span class="player-name ' + p1Class + '">' + m.player1 + '</span>' +
                        '<span class="vs">VS</span>' +
                        '<span class="player-name ' + p2Class + '">' + m.player2 + '</span>' +
                    '</div>' +
                    '<div class="badge ' + statusClass + '">' + m.status + '</div>' +
                '</a>';
            });

            html += '</div></div>';
        });

        roundsEl.innerHTML = html;

        // Toggle round sections
        roundsEl.querySelectorAll('.round-header').forEach(function(header) {
            header.addEventListener('click', function() {
                var matches = header.nextElementSibling;
                matches.classList.toggle('collapsed');
                header.classList.toggle('collapsed');
            });
        });
    }

    // Register button
    registerBtn.addEventListener('click', function() {
        var playerId = prompt('Enter your Player ID:');
        if (!playerId) return;

        fetch('/api/tournament/' + tournamentId + '/register', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ player_id: playerId })
        })
        .then(function(r) {
            if (!r.ok) return r.text().then(function(t) { throw new Error(t); });
            return r.json();
        })
        .then(function() {
            fetchTournament();
        })
        .catch(function(err) {
            alert('Registration failed: ' + err.message);
        });
    });

    fetchTournament();
})();

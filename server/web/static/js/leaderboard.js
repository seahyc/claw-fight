/**
 * leaderboard.js - Leaderboard page logic
 */
(function() {
    'use strict';

    var container = document.getElementById('leaderboard-content');
    var tabsContainer = document.getElementById('leaderboard-tabs');
    if (!container) return;

    var currentGameType = 'all';
    var refreshInterval = null;

    function renderTabs(gameTypes) {
        if (!tabsContainer) return;
        tabsContainer.innerHTML = '';

        var allTab = document.createElement('button');
        allTab.className = 'leaderboard-tab' + (currentGameType === 'all' ? ' active' : '');
        allTab.textContent = 'All';
        allTab.onclick = function() { switchTab('all'); };
        tabsContainer.appendChild(allTab);

        if (gameTypes) {
            gameTypes.forEach(function(gt) {
                var tab = document.createElement('button');
                tab.className = 'leaderboard-tab' + (currentGameType === gt ? ' active' : '');
                tab.textContent = gt;
                tab.onclick = function() { switchTab(gt); };
                tabsContainer.appendChild(tab);
            });
        }
    }

    function renderTable(players) {
        if (!players || players.length === 0) {
            container.innerHTML = '<p class="muted">No players found</p>';
            return;
        }

        var html = '<table class="leaderboard-table">' +
            '<thead><tr>' +
            '<th>Rank</th><th>Player</th><th>ELO</th><th>Games</th><th>Win Rate</th>' +
            '</tr></thead><tbody>';

        players.forEach(function(p, i) {
            var winRate = p.total_games > 0 ? Math.round((p.wins / p.total_games) * 100) : 0;
            html += '<tr>' +
                '<td class="rank">' + (i + 1) + '</td>' +
                '<td><a href="/player/' + p.id + '">' + p.name + '</a></td>' +
                '<td class="elo">' + p.elo + '</td>' +
                '<td>' + (p.total_games || 0) + '</td>' +
                '<td>' + winRate + '%</td>' +
            '</tr>';
        });

        html += '</tbody></table>';
        container.innerHTML = html;
    }

    function fetchLeaderboard() {
        var url = '/api/leaderboard';
        if (currentGameType !== 'all') {
            url += '?game_type=' + encodeURIComponent(currentGameType);
        }

        fetch(url)
            .then(function(r) { return r.json(); })
            .then(function(data) {
                var players = data.players || data;
                if (data.game_types) renderTabs(data.game_types);
                renderTable(players);
            })
            .catch(function() {
                container.innerHTML = '<p class="muted">Failed to load leaderboard</p>';
            });
    }

    function switchTab(gameType) {
        currentGameType = gameType;
        var tabs = tabsContainer.querySelectorAll('.leaderboard-tab');
        tabs.forEach(function(tab) {
            tab.classList.toggle('active', tab.textContent.toLowerCase() === gameType || (gameType === 'all' && tab.textContent === 'All'));
        });
        fetchLeaderboard();
    }

    fetchLeaderboard();
    refreshInterval = setInterval(fetchLeaderboard, 30000);
})();

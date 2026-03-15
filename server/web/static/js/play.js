/**
 * play.js - Player landing page logic
 */
(function() {
    'use strict';

    var playerName = localStorage.getItem('claw_player_name') || '';
    var playerId = localStorage.getItem('claw_player_id') || '';

    var nameInput = document.getElementById('player-name');
    var registerBtn = document.getElementById('register-btn');
    var registerStatus = document.getElementById('register-status');
    var joinPanel = document.getElementById('join-panel');
    var createPanel = document.getElementById('create-panel');
    var openPanel = document.getElementById('open-panel');
    var joinCode = document.getElementById('join-code');
    var joinBtn = document.getElementById('join-btn');
    var joinStatus = document.getElementById('join-status');
    var gameType = document.getElementById('game-type');
    var createBtn = document.getElementById('create-btn');
    var createStatus = document.getElementById('create-status');
    var openMatches = document.getElementById('open-matches');

    if (playerName) nameInput.value = playerName;

    // Pre-select game from query param
    var urlParams = new URLSearchParams(window.location.search);
    var preselectedGame = urlParams.get('game');
    if (preselectedGame && gameType) {
        gameType.value = preselectedGame;
    }

    function showLoggedIn() {
        registerStatus.textContent = 'Registered as: ' + playerName + ' (' + playerId.slice(0, 8) + '...)';
        registerStatus.className = 'play-status success';
        registerBtn.textContent = 'Update';
        joinPanel.style.display = '';
        createPanel.style.display = '';
        openPanel.style.display = '';
        fetchOpenMatches();
    }

    if (playerId && playerName) {
        showLoggedIn();
    }

    registerBtn.addEventListener('click', function() {
        var name = nameInput.value.trim();
        if (!name) {
            registerStatus.textContent = 'Please enter a name';
            registerStatus.className = 'play-status error';
            return;
        }
        registerBtn.disabled = true;
        fetch('/api/register', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ player_name: name, player_id: playerId || undefined })
        })
        .then(function(r) { return r.json(); })
        .then(function(data) {
            playerId = data.player_id;
            playerName = data.player_name;
            localStorage.setItem('claw_player_id', playerId);
            localStorage.setItem('claw_player_name', playerName);
            nameInput.value = playerName;
            showLoggedIn();
        })
        .catch(function(err) {
            registerStatus.textContent = 'Registration failed: ' + err.message;
            registerStatus.className = 'play-status error';
        })
        .finally(function() { registerBtn.disabled = false; });
    });

    joinBtn.addEventListener('click', function() {
        var code = joinCode.value.trim().toUpperCase();
        if (!code) {
            joinStatus.textContent = 'Enter a join code';
            joinStatus.className = 'play-status error';
            return;
        }
        joinBtn.disabled = true;
        joinStatus.textContent = 'Joining...';
        joinStatus.className = 'play-status';
        fetch('/api/match/join', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ code: code, player_id: playerId })
        })
        .then(function(r) {
            if (!r.ok) return r.text().then(function(t) { throw new Error(t); });
            return r.json();
        })
        .then(function(data) {
            window.location.href = '/play/' + data.match_id;
        })
        .catch(function(err) {
            joinStatus.textContent = 'Join failed: ' + err.message;
            joinStatus.className = 'play-status error';
        })
        .finally(function() { joinBtn.disabled = false; });
    });

    createBtn.addEventListener('click', function() {
        var gt = gameType.value;
        createBtn.disabled = true;
        createStatus.textContent = 'Creating match...';
        createStatus.className = 'play-status';
        fetch('/api/match', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ game_type: gt, player_id: playerId })
        })
        .then(function(r) {
            if (!r.ok) return r.text().then(function(t) { throw new Error(t); });
            return r.json();
        })
        .then(function(data) {
            createStatus.textContent = 'Match created! Code: ' + data.code + ' - Redirecting...';
            createStatus.className = 'play-status success';
            window.location.href = '/play/' + data.match_id;
        })
        .catch(function(err) {
            createStatus.textContent = 'Create failed: ' + err.message;
            createStatus.className = 'play-status error';
        })
        .finally(function() { createBtn.disabled = false; });
    });

    function fetchOpenMatches() {
        fetch('/api/matches/open')
            .then(function(r) { return r.json(); })
            .then(function(matches) {
                if (!matches || matches.length === 0) {
                    openMatches.innerHTML = '<p class="muted">No open matches</p>';
                    return;
                }
                openMatches.innerHTML = matches.map(function(m) {
                    return '<div class="open-match-card">' +
                        '<span class="open-match-type">' + (m.game_type || 'Unknown') + '</span>' +
                        '<span class="open-match-code">' + (m.code || '') + '</span>' +
                        '<span class="open-match-player">' + (m.player1 || 'Waiting...') + '</span>' +
                        '<button class="btn btn-small open-match-join" data-code="' + (m.code || '') + '">Join</button>' +
                    '</div>';
                }).join('');

                openMatches.querySelectorAll('.open-match-join').forEach(function(btn) {
                    btn.addEventListener('click', function() {
                        joinCode.value = btn.dataset.code;
                        joinBtn.click();
                    });
                });
            })
            .catch(function() {
                openMatches.innerHTML = '<p class="muted">Failed to load</p>';
            });
    }

    // Auto-refresh open matches every 5 seconds
    if (playerId) {
        setInterval(fetchOpenMatches, 5000);
    }
})();

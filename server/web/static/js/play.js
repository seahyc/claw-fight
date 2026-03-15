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
    var gameActions = document.getElementById('game-actions');
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
        registerStatus.textContent = 'Playing as ' + playerName;
        registerStatus.className = 'play-status success';
        registerBtn.textContent = 'Update';
        gameActions.style.display = '';
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
            registerStatus.textContent = 'Failed: ' + err.message;
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
        createStatus.textContent = 'Creating...';
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
            window.location.href = '/play/' + data.match_id;
        })
        .catch(function(err) {
            createStatus.textContent = 'Failed: ' + err.message;
            createStatus.className = 'play-status error';
        })
        .finally(function() { createBtn.disabled = false; });
    });

    function fetchOpenMatches() {
        fetch('/api/matches/open')
            .then(function(r) { return r.json(); })
            .then(function(matches) {
                if (!matches || matches.length === 0) {
                    openMatches.innerHTML = '';
                    return;
                }
                openMatches.innerHTML = matches.map(function(m) {
                    return '<div class="open-match-card">' +
                        '<span class="open-match-type">' + (m.game_type || '?') + '</span>' +
                        '<span class="open-match-code">' + (m.code || '') + '</span>' +
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
            .catch(function() {});
    }

    if (playerId) {
        setInterval(fetchOpenMatches, 5000);
    }
})();

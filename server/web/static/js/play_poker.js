/**
 * play_poker.js - Interactive poker UI for human players
 */
(function() {
    'use strict';

    var SUITS = { 'spades': '\u2660', 'hearts': '\u2665', 'diamonds': '\u2666', 'clubs': '\u2663',
                  's': '\u2660', 'h': '\u2665', 'd': '\u2666', 'c': '\u2663' };
    var RED_SUITS = ['hearts', 'diamonds', 'h', 'd'];

    var playerState = null;

    // Override spectator renderer
    var origRender = window.renderPokerBoard;
    window.renderPokerBoard = function(state) {
        if (window.PlayMatch && window.PlayMatch.playerId) {
            if (window.MatchViewer) window.MatchViewer.state = state;
            return;
        }
        if (origRender) origRender(state);
    };

    document.addEventListener('play_state_update', function(e) {
        var state = e.detail;
        if (!state || window.PlayMatch.gameType !== 'poker') return;
        playerState = state;
        renderPlayerView(state);
    });

    function isRed(suit) {
        return RED_SUITS.indexOf(suit) !== -1;
    }

    function renderCard(card) {
        var el = document.createElement('div');
        if (!card) {
            el.className = 'poker-card empty';
            return el;
        }
        if (card.hidden || card.face_down) {
            el.className = 'poker-card face-down';
            return el;
        }
        var red = isRed(card.suit);
        el.className = 'poker-card face-up' + (red ? ' red' : '');
        el.innerHTML = '<span class="poker-card-value">' + (card.value || card.rank || '?') + '</span>' +
            '<span class="poker-card-suit">' + (SUITS[card.suit] || card.suit || '') + '</span>';
        return el;
    }

    function renderPlayerView(state) {
        var boardEl = document.getElementById('game-board');
        if (!boardEl) return;
        boardEl.innerHTML = '';

        if (state.status === 'waiting') {
            boardEl.innerHTML = '<div class="phase-banner">Waiting for opponent to join...</div>';
            return;
        }

        var gameState = state.game_state || {};
        var table = document.createElement('div');
        table.className = 'poker-table';

        // Phase banner
        var phaseNames = {preflop: 'PRE-FLOP', flop: 'FLOP', turn: 'TURN', river: 'RIVER', showdown: 'SHOWDOWN'};
        var phaseBanner = document.createElement('div');
        phaseBanner.className = 'poker-phase-banner';
        var phaseText = phaseNames[gameState.phase] || (gameState.phase || '').toUpperCase();
        phaseBanner.textContent = phaseText;
        if (gameState.hand_number) {
            var handNum = document.createElement('span');
            handNum.className = 'poker-hand-number';
            handNum.textContent = ' \u2022 Hand #' + gameState.hand_number;
            phaseBanner.appendChild(handNum);
        }
        table.appendChild(phaseBanner);

        // Community cards
        var community = document.createElement('div');
        community.className = 'poker-community';
        var cards = gameState.community_cards || [];
        for (var i = 0; i < 5; i++) {
            community.appendChild(renderCard(cards[i] || null));
        }
        table.appendChild(community);

        // Pot
        var pot = document.createElement('div');
        pot.className = 'poker-pot';
        pot.innerHTML = 'Pot: <span class="poker-pot-amount">' + (gameState.pot || 0) + '</span>';
        table.appendChild(pot);

        // Players
        var playersArea = document.createElement('div');
        playersArea.className = 'poker-players-area';

        var players = gameState.players || [];
        var p1Name = (window.MatchViewer && MatchViewer.players.p1) || 'Player 1';
        var p2Name = (window.MatchViewer && MatchViewer.players.p2) || 'Player 2';
        var names = [p1Name, p2Name];

        for (var pi = 0; pi < 2; pi++) {
            var p = players[pi] || {};
            var playerDiv = document.createElement('div');
            playerDiv.className = 'poker-player';
            if (pi + 1 === window.PlayMatch.myPlayerNum) {
                playerDiv.className += ' is-me';
            }

            var nameEl = document.createElement('div');
            nameEl.className = 'poker-player-name';
            nameEl.textContent = names[pi] + (pi + 1 === window.PlayMatch.myPlayerNum ? ' (You)' : '');
            playerDiv.appendChild(nameEl);

            var chipsEl = document.createElement('div');
            chipsEl.className = 'poker-player-chips';
            chipsEl.textContent = 'Chips: ' + (p.chips !== undefined ? p.chips : '?');
            playerDiv.appendChild(chipsEl);

            if (p.current_bet !== undefined) {
                var betEl = document.createElement('div');
                betEl.className = 'poker-player-bet';
                betEl.textContent = 'Bet: ' + p.current_bet;
                playerDiv.appendChild(betEl);
            }

            var hand = document.createElement('div');
            hand.className = 'poker-hand';
            var handCards = p.hand || p.cards || [];
            for (var ci = 0; ci < 2; ci++) {
                hand.appendChild(renderCard(handCards[ci] || (handCards.length === 0 ? { face_down: true } : null)));
            }
            playerDiv.appendChild(hand);

            if (p.last_action) {
                var actionEl = document.createElement('div');
                actionEl.className = 'poker-player-action';
                actionEl.textContent = p.last_action;
                playerDiv.appendChild(actionEl);
            }

            playersArea.appendChild(playerDiv);
        }

        table.appendChild(playersArea);

        // Action buttons (only when it's our turn)
        if (window.PlayMatch.isMyTurn && state.status !== 'completed') {
            var actions = document.createElement('div');
            actions.className = 'poker-actions';

            var foldBtn = document.createElement('button');
            foldBtn.className = 'btn btn-danger';
            foldBtn.textContent = 'Fold';
            foldBtn.addEventListener('click', function() { sendPokerAction('fold'); });
            actions.appendChild(foldBtn);

            var checkBtn = document.createElement('button');
            checkBtn.className = 'btn btn-secondary';
            checkBtn.textContent = 'Check';
            checkBtn.addEventListener('click', function() { sendPokerAction('check'); });
            actions.appendChild(checkBtn);

            var callBtn = document.createElement('button');
            callBtn.className = 'btn btn-primary';
            callBtn.textContent = 'Call';
            callBtn.addEventListener('click', function() { sendPokerAction('call'); });
            actions.appendChild(callBtn);

            var raiseGroup = document.createElement('div');
            raiseGroup.className = 'poker-raise-group';
            var raiseInput = document.createElement('input');
            raiseInput.type = 'number';
            raiseInput.className = 'pixel-input raise-input';
            raiseInput.placeholder = 'Amount';
            raiseInput.min = '1';
            raiseInput.id = 'raise-amount';
            raiseGroup.appendChild(raiseInput);

            var raiseBtn = document.createElement('button');
            raiseBtn.className = 'btn btn-primary';
            raiseBtn.textContent = 'Raise';
            raiseBtn.addEventListener('click', function() {
                var amount = parseInt(document.getElementById('raise-amount').value);
                if (isNaN(amount) || amount <= 0) {
                    alert('Enter a valid raise amount');
                    return;
                }
                sendPokerAction('raise', { amount: amount });
            });
            raiseGroup.appendChild(raiseBtn);
            actions.appendChild(raiseGroup);

            table.appendChild(actions);
        } else if (!window.PlayMatch.isMyTurn && state.status !== 'completed' && state.status !== 'waiting') {
            var waitBanner = document.createElement('div');
            waitBanner.className = 'poker-wait-banner';
            waitBanner.textContent = "Opponent's turn...";
            table.appendChild(waitBanner);
        }

        // Showdown result
        if (gameState.showdown && gameState.showdown.hands) {
            var showdownBanner = document.createElement('div');
            showdownBanner.className = 'poker-showdown-result';
            var hands = gameState.showdown.hands;
            var handKeys = Object.keys(hands);
            var parts = [];
            for (var si = 0; si < handKeys.length; si++) {
                var h = hands[handKeys[si]];
                var pName = names[si] || handKeys[si];
                parts.push(pName + ': ' + (h.rank_name || 'Unknown'));
            }
            showdownBanner.textContent = parts.join(' vs ');
            table.appendChild(showdownBanner);
        }

        boardEl.appendChild(table);
    }

    function sendPokerAction(actionType, data) {
        var allBtns = document.querySelectorAll('.poker-actions button');
        allBtns.forEach(function(b) { b.disabled = true; });

        window.PlayMatch.sendAction(actionType, data || {})
            .then(function(result) {
                if (!result.success) {
                    alert('Action failed: ' + (result.message || 'Unknown error'));
                    allBtns.forEach(function(b) { b.disabled = false; });
                }
            })
            .catch(function(err) {
                alert('Error: ' + err.message);
                allBtns.forEach(function(b) { b.disabled = false; });
            });
    }
})();

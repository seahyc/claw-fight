/**
 * board_poker.js - Poker board renderer
 */
(function() {
    'use strict';

    var SUITS = { 'spades': '\u2660', 'hearts': '\u2665', 'diamonds': '\u2666', 'clubs': '\u2663',
                  's': '\u2660', 'h': '\u2665', 'd': '\u2666', 'c': '\u2663' };
    var RED_SUITS = ['hearts', 'diamonds', 'h', 'd'];

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

    window.renderPokerBoard = function(state) {
        var boardEl = document.getElementById('game-board');
        if (!boardEl || !state) return;

        boardEl.innerHTML = '';

        var table = document.createElement('div');
        table.className = 'poker-table';

        // Phase banner
        var phaseNames = {preflop: 'PRE-FLOP', flop: 'FLOP', turn: 'TURN', river: 'RIVER', showdown: 'SHOWDOWN'};
        var phaseBanner = document.createElement('div');
        phaseBanner.className = 'poker-phase-banner';
        var phaseText = phaseNames[state.phase] || (state.phase || '').toUpperCase();
        phaseBanner.textContent = phaseText;
        if (state.hand_number) {
            var handNum = document.createElement('span');
            handNum.className = 'poker-hand-number';
            handNum.textContent = ' \u2022 Hand #' + state.hand_number;
            phaseBanner.appendChild(handNum);
        }
        table.appendChild(phaseBanner);

        // Community cards
        var community = document.createElement('div');
        community.className = 'poker-community';
        var cards = state.community_cards || [];
        for (var i = 0; i < 5; i++) {
            community.appendChild(renderCard(cards[i] || null));
        }
        table.appendChild(community);

        // Pot
        var pot = document.createElement('div');
        pot.className = 'poker-pot';
        pot.innerHTML = 'Pot: <span class="poker-pot-amount">' + (state.pot || 0) + '</span>';
        table.appendChild(pot);

        // Players
        var playersArea = document.createElement('div');
        playersArea.className = 'poker-players-area';

        var players = state.players || [];
        var p1Name = (window.MatchViewer && MatchViewer.players.p1) || 'Player 1';
        var p2Name = (window.MatchViewer && MatchViewer.players.p2) || 'Player 2';
        var names = [p1Name, p2Name];

        for (var pi = 0; pi < 2; pi++) {
            var p = players[pi] || {};
            var playerDiv = document.createElement('div');
            playerDiv.className = 'poker-player';

            var nameEl = document.createElement('div');
            nameEl.className = 'poker-player-name';
            nameEl.textContent = names[pi];
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

        // Showdown result
        if (state.showdown && state.showdown.hands) {
            var showdownBanner = document.createElement('div');
            showdownBanner.className = 'poker-showdown-result';
            var hands = state.showdown.hands;
            var handKeys = Object.keys(hands);
            var parts = [];
            for (var si = 0; si < handKeys.length; si++) {
                var h = hands[handKeys[si]];
                var pName = (window.MatchViewer && window.MatchViewer.players['p' + (si+1)]) || handKeys[si];
                parts.push(pName + ': ' + (h.rank_name || 'Unknown'));
            }
            showdownBanner.textContent = parts.join(' vs ');
            table.appendChild(showdownBanner);
        }

        boardEl.appendChild(table);
    };
})();

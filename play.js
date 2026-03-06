#!/usr/bin/env node
const WebSocket = require('ws');
const readline = require('readline');

const rl = readline.createInterface({ input: process.stdin, output: process.stdout });

let ws = null;
let playerId = null;
let matchId = null;

function connect() {
  return new Promise((resolve, reject) => {
    ws = new WebSocket('ws://127.0.0.1:7429/ws');
    ws.on('open', resolve);
    ws.on('error', reject);
    ws.on('message', handleMessage);
  });
}

function handleMessage(data) {
  const msg = JSON.parse(data);
  console.log('\n[MSG]', msg.type);

  if (msg.type === 'registered') {
    playerId = msg.player_id;
    console.log(`✓ Registered as ${msg.player_id}`);
  } else if (msg.type === 'match_found') {
    matchId = msg.match_id;
    console.log(`✓ Match found: ${matchId}`);
    console.log(`  Game: ${msg.game_type}`);
    console.log(`  Opponent: ${msg.opponent_name}`);
    showPrompt();
  } else if (msg.type === 'your_turn') {
    console.log(`\n🎯 YOUR TURN (Phase: ${msg.phase})`);
    if (msg.available_actions) {
      console.log(`   Actions: ${msg.available_actions.join(', ')}`);
    }
    showPrompt();
  } else if (msg.type === 'game_over') {
    console.log(`\n✓ GAME OVER`);
    console.log(`  Result: ${msg.result}`);
    console.log(`  Your score: ${msg.your_score}`);
    console.log(`  Opponent: ${msg.opponent_score}`);
    process.exit(0);
  } else if (msg.type === 'action_result') {
    console.log(`  Result: ${msg.message}`);
  }
}

function send(message) {
  ws.send(JSON.stringify(message));
}

function showPrompt() {
  rl.question('\n> ', (input) => {
    const [cmd, ...args] = input.split(' ');

    switch (cmd.toLowerCase()) {
      case 'cooperate':
      case 'c':
        send({ type: 'action', match_id: matchId, action_type: 'cooperate' });
        break;
      case 'defect':
      case 'd':
        send({ type: 'action', match_id: matchId, action_type: 'defect' });
        break;
      case 'status':
      case 's':
        send({ type: 'get_state', match_id: matchId });
        break;
      case 'quit':
      case 'q':
        ws.close();
        process.exit(0);
        break;
      default:
        console.log('Commands: cooperate (c), defect (d), status (s), quit (q)');
        showPrompt();
    }
  });
}

async function main() {
  try {
    console.log('🎮 Claw Fight - Direct Player');
    console.log('Connecting...');

    await connect();
    console.log('✓ Connected');

    const name = process.argv[2] || 'DIRECT_PLAYER';
    send({ type: 'register', player_name: name });

    console.log(`Registering as: ${name}`);
    console.log('Waiting for match...');

  } catch (err) {
    console.error('Connection failed:', err.message);
    process.exit(1);
  }
}

main();

(function (window) {
    "use strict";

    var serverPath = '{{ .ServerPath }}';

    function App() {
        console.log("Starting...");

        $('div.main-box').hide();
        $('div.loading-mode').show();

        window.apis_ready.done(this.onApiReady.bind(this));

        $('button.start-game').click(this.gameStartClick.bind(this));
        $('div.pick-mode form.proposal button').click(this.commitProposal.bind(this));
        $('div.voting-mode form.vote button').click(this.commitVote.bind(this));
        $('div.mission-mode form.mission button').click(this.commitMission.bind(this));

        var that = this;
        $(document).keypress(function (e) {if (e.which == 172) {$('div.debug').show();}});
        $('div.debug button#start').click(function() { that.startInterval() });
        $('div.debug button#stop').click(function() { that.stopInterval() });
        $('div.debug button#refresh').click(function() { that.fetchGameState() });
    }

    App.prototype.apiready = false;

    App.prototype.onApiReady = function (event) {
        if (this.apiready) {
            return;
        }
        if (gapi.hangout.isApiReady()) {
            console.log("API Ready");

            this.apiready = true;
            this.participant_ids = null;
            this.namecache = {};
            this.myid = gapi.hangout.getLocalParticipantId();
            this.gameid = null;
            this.leader = null;
            this.gamestate = 'joining';
            this.this_mission = null;
            this.this_proposal = null;
            this.ui = {};
            this.fetchajax = null;
            this.$msgbox = $("<div/>");
            this.$msgbox.dialog({autoOpen: false, modal: true, width: 250});

            gapi.hangout.onParticipantsChanged.add(this.onParticipantsChanged.bind(this));

            gapi.hangout.data.onStateChanged.add(this.checkState.bind(this));

            this.auth_got = false;
            window.auth_callback_gapi = this.authDone.bind(this);
            gapi.auth.signIn({
                'accesstype': 'online',
                'clientid': '{{ .ClientID }}',
                'cookiepolicy': 'single_host_origin',
                'callback': 'auth_callback_gapi',
                'redirecturi': 'postmessage',
                'scope': 'https://www.googleapis.com/auth/plus.me',
            });

            this.interval = null;
        }
    };

    App.prototype.startInterval = function() {
        if (this.interval === null) {
            this.interval = window.setInterval(this.timer.bind(this), 5000);
        }
    };

    App.prototype.stopInterval = function() {
        if (this.interval !== null) {
            window.clearInterval(this.interval);
            this.interval = null;
        }
    };

    App.prototype.api = function(call, args) {
        return $.ajax({
            type: 'POST',
            url: serverPath + call,
            headers: { 'x-csrf-token': '{{ .State }}' },
            contentType: 'application/json; charset=utf-8',
            data: JSON.stringify(args),
            dataType: 'json',
            xhrFields: {
                withCredentials: true
            },
        });
    };

    App.prototype.authDone = function(authResult) {
        if (this.auth_got) {
            return;
        }
        this.auth_got = true;

        this.api('auth/token',
                 {token: gapi.auth.getToken().access_token,
                  myid: gapi.hangout.getLocalParticipantId(),
                  hangout: gapi.hangout.getHangoutId(),
                 })
            .done(this.gameReady.bind(this));
    };

    App.prototype.gameReady = function(result) {
        this.ui = {};
        this.gamestate = 'start';
        $('div.main-box').hide();
        $('button.start-game').prop('disabled', true);
        this.startMode();
    };

    App.prototype.showGamePlayers = function() {
        if (this.gamestate != 'start' && this.gamestate != 'gameover') {
            return;
        }

        this.ui.$gameplayers.empty();

        var participant_ids = {};
        var participants = gapi.hangout.getParticipants();

        var foundme = false;
        for (var i = 0; i < participants.length; i++) {
            var $player = $("<div/>");
            var text = participants[i].person.displayName;
            if (participants[i].id == this.myid) {
                text += " (me)";
            }
            $player.text(text);
            this.ui.$gameplayers.append($player);

            participant_ids[participants[i].id] = participants[i].person.id;
            if (participants[i].id == this.myid) {
                foundme = true;
            }
        }

        if (foundme) {
            this.participant_ids = participant_ids;
            this.participant_count = participants.length;
        }
        else {
            console.log("WTF: didn't find me in", participants, gapi.hangout.getParticipants());
        }
    };

    App.prototype.gameStartClick = function() {
        if (this.participant_ids === null) {
            return false;
        }

        this.ui.$start_button.prop('disabled', true);

        var goodcards = this.ui.$goodcards.children(".selected").map(function() {return $(this).data("label")}).get();
        var evilcards = this.ui.$evilcards.children(".selected").map(function() {return $(this).data("label")}).get();

        for (var i = goodcards.length; i < this.setup_good_count; i++) {
            goodcards.push("Good");
        }
        for (var i = evilcards.length; i < this.setup_evil_count; i++) {
            evilcards.push("Evil");
        }

        var that = this;

        this.api('game/start',
                 { players: this.participant_ids,
                   cards: goodcards.concat(evilcards),
                 }
                ).done(this.handleGameState.bind(this))
            .fail(function() {that.ui.$start_button.prop('disabled', false)});
    };

    App.prototype.joinGame = function(gameid) {
        this.api('game/join', {}).done(this.handleGameState.bind(this));
    };

    App.prototype.gameStart = function(gameid) {
        this.this_mission = null;
        this.this_proposal = null;
        this.leader = null;
        this.gameid = gameid;
        this.renderedmissions = -1;

        this.revealRoles();
        $('.info-box').show();
        this.startInterval();
    };

    App.prototype.startMode = function() {
        this.ui.$start_button = $('button.start-game');
        this.ui.$gameplayers = $('div.start-mode div.gameplayers');
        this.ui.$cardchoice = $('div.cardchoice');
        this.ui.$goodcards = this.ui.$cardchoice.find('.card-box.good');
        this.ui.$genericgood = this.ui.$cardchoice.find('.generic.good');
        this.ui.$evilcards = this.ui.$cardchoice.find('.card-box.evil');
        this.ui.$genericevil = this.ui.$cardchoice.find('.generic.evil');
        this.showGamePlayers();

        this.ui.$cardchoice.hide();
        $('div.start-mode').show();

        this.loadSetup();
    };

    App.prototype.loadSetup = function() {
        if (this.participant_ids === null || this.gamestate != 'start') {
            return;
        }

        this.setup_players = this.participant_count;
        if (this.setup_players < 5) {
            this.setup_players = 5;
        }

        if (this.loadsetupajax) {
            this.loadsetupajax.abort();
        }
        this.loadsetupajax = this.api('game/setup', { players: this.setup_players }).done(this.handleGameSetup.bind(this));
    };

    App.prototype.handleGameSetup = function(msg) {
        this.loadsetupajax = null;
        if (this.gamestate != 'start') {
            return;
        }

        this.setup_evil_count = msg.setup.spies;
        this.setup_good_count = this.setup_players - this.setup_evil_count;

        // Remove the generic cards
        var goodcards = msg.good_cards.filter(function(e) {return e != "Good"});
        var evilcards = msg.evil_cards.filter(function(e) {return e != "Evil"});
        goodcards.sort();
        evilcards.sort();

        this.ui.$goodcards.empty();
        this.ui.$evilcards.empty();

        for (var i = 0; i < goodcards.length; i++) {
            var $card = $("<div class='card good unselected'/>");
            $card.text(goodcards[i]);
            $card.data("label", goodcards[i]);
            $card.prepend($("<span class='card-icon ui-icon ui-icon-close'>"));
            $card.click(this.clickCard.bind(this, $card));
            this.ui.$goodcards.append($card);
        }

        for (var i = 0; i < evilcards.length; i++) {
            var $card = $("<div class='card evil unselected'/>");
            $card.text(evilcards[i]);
            $card.data("label", evilcards[i]);
            $card.prepend($("<span class='card-icon ui-icon ui-icon-close'>"));
            $card.click(this.clickCard.bind(this, $card));
            this.ui.$evilcards.append($card);
        }

        this.updateGameSetup();

        this.ui.$cardchoice.show();
    };

    App.prototype.updateGameSetup = function(msg) {
        if (this.gamestate != 'start') {
            return;
        }

        var good_count = this.ui.$goodcards.children(".selected").length;
        var evil_count = this.ui.$evilcards.children(".selected").length;

        var generic_good = this.setup_good_count - good_count;
        var generic_evil = this.setup_evil_count - evil_count;

        $(".start-mode button.start-game").prop('disabled', false);

        if (generic_good < 0) {
            this.ui.$genericgood.text("Too many!");
            $(".start-mode button.start-game").prop('disabled', true);
        }
        else if (generic_good == 0) {
            this.ui.$genericgood.text("");
        }
        else {
            this.ui.$genericgood.text("...plus " + generic_good + " Good card" + (generic_good == 1 ? "" : "s"));
        }

        if (generic_evil < 0) {
            this.ui.$genericevil.text("Too many!");
            $(".start-mode button.start-game").prop('disabled', true);
        }
        else if (generic_evil == 0) {
            this.ui.$genericevil.text("");
        }
        else {
            this.ui.$genericevil.text("...plus " + generic_evil + " Evil card" + (generic_evil == 1 ? "" : "s"));
        }
    };

    App.prototype.clickCard = function($card, e) {
        if ($card.hasClass("selected")) {
            $card.removeClass("selected").addClass("unselected");
            $card.find(".card-icon").removeClass("ui-icon-check").addClass("ui-icon-close");
        }
        else {
            $card.removeClass("unselected").addClass("selected");
            $card.find(".card-icon").removeClass("ui-icon-close").addClass("ui-icon-check");
        }
        this.updateGameSetup();
    };

    App.prototype.resetPickMode = function() {
        this.ui.$pickbox.hide();
        this.ui.$commitproposal.prop('disabled', false);
        this.ui.$pick.empty();
    };

    App.prototype.pickMode = function() {
        this.ui.$count = $('div.pick-mode span.playercount');

        this.ui.$pickbox = $('div.pick-mode form.proposal');
        this.ui.$pick = this.ui.$pickbox.children('div.proposal');
        this.ui.$commitproposal = this.ui.$pickbox.children('button');

        this.resetMode = this.resetPickMode;
        this.resetMode();

        $('div.pick-mode').show();
    };

    App.prototype.resetVotingMode = function() {
        this.ui.$missionplayers.empty();
        this.ui.$commitvote.prop('disabled', false);
        this.ui.$approve.prop('disabled', false);
        this.ui.$reject.prop('disabled', false);
        this.ui.$approve.prop('checked', false);
        this.ui.$reject.prop('checked', false);
    };

    App.prototype.votingMode = function() {
        this.ui.$missionplayers = $('div.voting-mode div.missionplayers');

        this.ui.$pickbox = $('div.voting-mode form.vote');
        this.ui.$approve = this.ui.$pickbox.children('input.approve');
        this.ui.$reject = this.ui.$pickbox.children('input.reject');
        this.ui.$commitvote = this.ui.$pickbox.children('button');

        this.resetMode = this.resetVotingMode;
        this.resetMode();

        $('div.voting-mode').show();
    };

    App.prototype.resetMissionMode = function() {
        this.ui.$missionplayers.empty();
        this.ui.$pickbox.hide();
        this.ui.$commitmission.prop('disabled', false);
        this.ui.$success.prop('disabled', false);
        this.ui.$failure.prop('disabled', false);
        this.ui.$success.prop('checked', false);
        this.ui.$failure.prop('checked', false);
    };

    App.prototype.missionMode = function() {
        this.ui.$missionplayers = $('div.mission-mode div.missionplayers');

        this.ui.$pickbox = $('div.mission-mode form.mission');
        this.ui.$success = this.ui.$pickbox.children('input.success');
        this.ui.$failure = this.ui.$pickbox.children('input.fail');
        this.ui.$commitmission = this.ui.$pickbox.children('button');

        this.resetMode = this.resetMissionMode;
        this.resetMode();

        $('div.mission-mode').show();
    };

    App.prototype.resetGameoverMode = function() {
        this.ui.$result.text('');
        this.ui.$comment.text('');
        this.ui.$playercards.empty();
        $('button.start-game').prop('disabled', false);
    };

    App.prototype.gameoverMode = function() {
        this.ui.$result = $('div.gameover-mode span.result');
        this.ui.$comment = $('div.gameover-mode span.comment');
        this.ui.$playercards = $('div.gameover-mode div.playercards');
        this.ui.$start_button = $('button.start-game');
        this.ui.$gameplayers = $('div.gameover-mode div.gameplayers');

        this.resetMode = this.resetGameoverMode;
        this.resetMode();
        this.showGamePlayers();

        $('div.gameover-mode').show();
    };

    App.prototype.changeMode = function(state) {
        console.log("Set UI mode: " + state);

        this.ui = {};
        this.gamestate = state;
        gapi.hangout.data.submitDelta({gamestate: state});

        $('div.main-box').hide();

        if (state == 'picking') {
            this.pickMode();
        }
        else if (state == 'voting') {
            this.votingMode();
        }
        else if (state == 'mission') {
            this.missionMode();
        }
        else if (state == 'gameover') {
            this.gameoverMode();
        }

        this.ui.$tableplayers = $('div.table-order div.players');
        this.ui.$lastproposal = $('div.last-proposal');
        this.ui.$lastproposalplayers = $('div.last-proposal div.players');
        this.ui.$leader = $('div.game-status span.leader');
        this.ui.$thismission = $('div.game-status span.thismission');
        this.ui.$thisproposal = $('div.game-status span.thisproposal');
        this.ui.$missionstatus = $('div.mission-results div.mission-status');
    }

    App.prototype.commitProposal = function() {
        if (this.gamestate != 'picking' || this.leader != this.mypos) {
            return false;
        }

        console.log("commitProposal");
        var $selected = $('input:checked', '#proposal');
        console.log($selected.length, this.missionsize);
        if ($selected.length != this.missionsize) {
            return false;
        }

        var that = this;

        var players = [];
        $selected.each(function(i) { players.push( parseInt($(this).val()) ) });

        console.log(players);

        this.ui.$commitproposal.prop('disabled', true);
        this.ui.$pick.children('input').prop('disabled', true);

        this.api('game/propose',
                 { mission: this.this_mission,
                   proposal: this.this_proposal,
                   players: players
                 }
                ).done(this.handleGameState.bind(this))
            .fail(function() {
                that.ui.$commitproposal.prop('disabled', false)
                that.ui.$pick.children('input').prop('disabled', false);
            });

        return false;
    };

    App.prototype.commitVote = function() {
        if (this.gamestate != 'voting') {
            return false;
        }

        var $selected = $('input[name=vote]:checked', '#vote');
        if ($selected.length != 1) {
            return false;
        }

        this.ui.$commitvote.prop('disabled', true);
        this.ui.$approve.prop('disabled', true);
        this.ui.$reject.prop('disabled', true);

        var vote = $selected.val();
        var that = this;

        this.api('game/vote',
                 { mission: this.this_mission,
                   proposal: this.this_proposal,
                   vote: vote
                 }
                ).done(this.handleGameState.bind(this))
            .fail(function() {
                that.ui.$commitvote.prop('disabled', false)
                that.ui.$approve.prop('disabled', false);
                that.ui.$reject.prop('disabled', false);
            });

        return false;
    };

    App.prototype.commitMission = function() {
        if (this.gamestate != 'mission') {
            return false;
        }

        var $selected = $('input[name=action]:checked', '#mission');
        if ($selected.length != 1) {
            return false;
        }

        this.ui.$commitmission.prop('disabled', true);
        this.ui.$success.prop('disabled', true);
        this.ui.$failure.prop('disabled', true);

        var that = this;

        this.api('game/mission',
                 { mission: this.this_mission,
                   action: $selected.val()
                 }
                ).done(this.handleGameState.bind(this))
            .fail(function() {
                that.ui.$commitmission.prop('disabled', false)
                that.ui.$success.prop('disabled', false)
                that.ui.$failure.prop('disabled', false)
            });
        return false;
    };

    App.prototype.becomeLeader = function() {
        for (var i = 0; i < this.players.length; i++) {
            var id = this.players[i];
            var $box = $("<div/>");
            var $label = $("<label/>");
            var $input = $("<input type='checkbox'/>");
            $input.attr('value', i);
            $box.append($label);
            $label.text(this.playerName(i))
            $label.prepend($input);
            this.ui.$pick.append($box);
        }

        this.ui.$pickbox.show();
    };

    App.prototype.checkState = function (event) {
        var state = gapi.hangout.data.getState();
        if (state.gameid != this.gameid) {
            console.log("hangout state said we need to join the game");
            this.joinGame();
        }
        if (state.gamestate != this.gamestate) {
            this.refreshGame();
        }
    };

    App.prototype.onParticipantsChanged = function (event) {
        this.showGamePlayers();
        this.loadSetup();
    };

    App.prototype.revealRoles = function() {
        this.api('game/reveal', {}).done(this.handleReveal.bind(this));
    };

    App.prototype.handleReveal = function(msg) {
        this.$msgbox.empty();
        this.$msgbox.dialog('option', 'title', 'You can see...');

        var card_counts = {};
        for (var i = 0; i < this.gamesetup.cards.length; i++) {
            var label = this.gamesetup.cards[i];
            card_counts[label] = (card_counts[label] || 0) + 1;
        }

        var cards = [];
        for (var card in card_counts) {
            if (card_counts[card] == 1) {
                cards.push(card);
            }
            else {
                cards.push(card + "\xa0x" + card_counts[card]);
            }
        }
        cards.sort();
        var $setupbox = $("<div class='revealsetup'/>");
        $setupbox.text("Cards in the game")
        var $cardlist = $("<div class='cards'/>");
        $setupbox.append($cardlist);
        for (var i = 0; i < cards.length; i++) {
            var $cardbox = $("<div class='card'/>");
            $cardbox.text(cards[i]);
            $cardlist.append($cardbox);
        }

        this.$msgbox.append($setupbox);

        for (var i = 0; i < msg.length; i++) {
            var $box = $("<div class='reveal'/>");
            $box.text(msg[i].label);
            var $list = $("<div class='players'/>");
            this.renderPlayers(msg[i].players, {}, null, $list);
            $box.append($list);
            this.$msgbox.append($box);
        }

        this.$msgbox.dialog("option", "dialogClass", "reveal-dialog");
        this.$msgbox.dialog("open");
    };

    App.prototype.fetchGameState = function() {
        if (this.fetchajax) {
            this.fetchajax.abort();
        }
        this.fetchajax = this.api('game/state', {}).done(this.handleGameState.bind(this));
    };

    var ai_re = /^ai_(\d+)$/;

    var do_playerNameById = function (id) {
        var result = ai_re.exec(id);
        if (result === null) {
            var participant = gapi.hangout.getParticipantById(id);
            if (participant === null) {
                // Note that this only happens for players that we've
                // never seen in the namecache
                return '<absent player>';
            }
            else {
                return participant.person.displayName;
            }
        }
        else {
            return '<AI ' + result[1] + '>';
        }
    };

    App.prototype.playerNameById = function (id) {
        if (!(id in this.namecache)) {
            this.namecache[id] = do_playerNameById(id);
        }
        return this.namecache[id];
    };

    App.prototype.playerName = function (pos) {
        return this.playerNameById(this.players[pos]);
    };

    App.prototype.renderPlayers = function (players, data, icons, $list) {
        $list.empty();

        var includesme = false;

        for (var i = 0; i < players.length; i++) {
            var $player = $("<div/>");
            var $iconbox = $("<div class='player-icon'/>");
            var text = this.playerName(players[i]);
            if (i in data) {
                text += ': ' + data[i];
            }
            $player.text(text);
            if (icons) {
                $player.prepend($iconbox);
                if (players[i] in icons) {
                    var $icon = $("<span class='player-icon ui-icon'></span>");
                    $icon.addClass(icons[players[i]]);
                    $iconbox.append($icon);
                    //text = ' ' + text;
                }
            }
            $list.append($player);
            if (players[i] == this.mypos) {
                includesme = true;
            }
        }

        return includesme;
    };

    App.prototype.renderMissions = function (setup, results) {
        if (this.renderedmissions == results.length) {
            return;
        }

        this.ui.$missionstatus.empty();
        var missions = [];
        for (var i = 0; i < setup.missions.length; i++) {
            var $missionbox = $("<div class='mission'/>");
            var text = setup.missions[i].size;
            var hint = "Mission " + (i+1) + " will have " + setup.missions[i].size + " players"
            if (setup.missions[i].fails_allowed > 0) {
                text += "*";
                hint += ", and will only fail if " + setup.missions[i].fails_allowed + " fail cards are present";
            }
            $missionbox.text(text);
            $missionbox.attr('title', hint);
            missions.push($missionbox);
            this.ui.$missionstatus.append($missionbox);
            // This is vital to word-splitting for the css justification magic
            this.ui.$missionstatus.append(' ');

            $missionbox.tooltip({tooltipClass: 'mission-tooltip',
                                 position: {my: "center top", at: "center bottom+10", collision: "fit", within: $("#main")},
                                });
        }
        this.ui.$missionstatus.append($("<span class='stretch'/>"));
        for (var i = 0; i < results.length; i++) {
            var $mission = missions[results[i].mission];
            if (results[i].fails > results[i].fails_allowed) {
                $mission.addClass('failed');
            }
            else {
                $mission.addClass('success');
            }
            var players = results[i].players.map(this.playerName.bind(this));
            var text = players.join(", ") + ", Leader: " + this.playerName(results[i].leader);
            if (results[i].fails > 0) {
                text += " (" + results[i].fails + " fails)"
            }
            $mission.attr('title', text);
        }
        missions[this.this_mission - 1].addClass('current');
        this.renderedmissions = results.length;
    };

    App.prototype.handleGameState = function (msg) {
        console.log(msg);
        this.fetchajax = null;
        this.players = msg.general.players;
        this.mypos = this.players.indexOf(this.myid);
        this.results = msg.general.results;
        this.votes = msg.general.votes;
        this.gamesetup = msg.general.setup;

        if (this.gameid != msg.general.gameid) {
            console.log("/state said we need to change games");
            this.gameStart(msg.general.gameid);
        }
        gapi.hangout.data.submitDelta({gameid: msg.general.gameid});

        if (this.gamestate != msg.general.state) {
            this.changeMode(msg.general.state);
        }
        else if (this.this_mission != msg.general.this_mission || this.this_proposal != msg.general.this_proposal) {
            // We've gone through an entire cycle between refreshes,
            // so while we're in the same state, we need to reset it
            this.resetMode();
            this.leader = null;
        }
        this.this_mission = msg.general.this_mission;
        this.this_proposal = msg.general.this_proposal;

        var lastvote = {};
        var last_votes;
        if (msg.general.votes.length > 0) {
            last_votes = msg.general.votes[msg.general.votes.length - 1];
        }

        if (last_votes) {
            for (var i = 0; i < last_votes.votes.length; i++) {
                if (last_votes.votes[i]) {
                    lastvote[i] = "approve";
                }
                else {
                    lastvote[i] = "reject";
                }
            }

            if (msg.general.state != 'mission') {
                var icons = {};
                icons[last_votes.leader] = 'ui-icon-star';
                this.renderPlayers(last_votes.players, {}, icons, this.ui.$lastproposalplayers);
                this.ui.$lastproposal.show();
            }
            else {
                this.ui.$lastproposal.hide();
            }
        }
        else {
            this.ui.$lastproposal.hide();
        }

        var tableplayers = [];
        for (var i = 0; i < msg.general.players.length; i++) {
            tableplayers.push(i);
        }
        var readyicons = {};

        this.ui.$leader.text(this.playerName(msg.general.leader));
        this.ui.$thismission.text(msg.general.this_mission);
        this.ui.$thisproposal.text(msg.general.this_proposal);

        this.renderMissions(msg.general.setup, msg.general.mission_results);

        if (msg.general.state == 'picking') {
            readyicons[msg.general.leader] = 'ui-icon-comment';
            if (msg.general.leader == this.mypos && this.leader != this.mypos) {
                this.becomeLeader();
            }
            this.leader = msg.general.leader;

            this.missionsize = msg.mission_size;

            this.ui.$count.text(msg.mission_size);
        }
        else if (msg.general.state == 'voting') {
            for (var i = 0; i < msg.voted_players.length; i++) {
                var icon;
                if (msg.voted_players[i]) {
                    icon = 'ui-icon-check';
                }
                else {
                    icon = 'ui-icon-comment';
                }
                readyicons[i] = icon;
            }

            var icons = {};
            icons[msg.general.leader] = 'ui-icon-star';
            this.renderPlayers(msg.mission_players, {}, icons, this.ui.$missionplayers);
        }
        else if (msg.general.state == 'mission') {
            for (var i = 0; i < msg.mission_players.length; i++) {
                var icon;
                if (msg.acted_players[i]) {
                    icon = 'ui-icon-check';
                }
                else {
                    icon = 'ui-icon-comment';
                }
                readyicons[msg.mission_players[i]] = icon;
            }

            var icons = {};
            icons[msg.general.leader] = 'ui-icon-star';
            var mymission = this.renderPlayers(msg.mission_players, {}, icons, this.ui.$missionplayers);

            if (mymission) {
                this.ui.$success.prop('disabled', !msg.allow_actions['Success']);
                this.ui.$failure.prop('disabled', !msg.allow_actions['Failure']);
                this.ui.$pickbox.show();
            }
        }
        else if (msg.general.state == 'gameover') {
            this.ui.$result.text(msg.result);
            this.ui.$comment.text(msg.comment);

            this.renderPlayers(tableplayers, msg.cards, {}, this.ui.$playercards);
            this.stopInterval();
        }

        this.renderPlayers(tableplayers, lastvote, readyicons, this.ui.$tableplayers);
    };

    App.prototype.inrefresh = false;
    App.prototype.refreshGame = function() {
        // Debounce this to make recursive calls safe, so anything can
        // just call refreshGame and be confident that a full update
        // occurred
        if (this.inrefresh || this.gameid === null) {
            return;
        }
        this.inrefresh = true;

        this.checkState();
        this.fetchGameState();

        this.inrefresh = false;
    };

    App.prototype.timer = function() {
        this.refreshGame();
    };

    var app = new App();
    window.app = app;
}(window));

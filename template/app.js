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

            var that = this;
            $('div.debug button#start').click(function() { that.startInterval() });
            $('div.debug button#stop').click(function() { that.stopInterval() });
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

    App.prototype.authDone = function(authResult) {
        if (this.auth_got) {
            return;
        }
        this.auth_got = true;

        $.ajax({
            type: 'POST',
            url: serverPath + 'auth/token?state={{ .State }}',
            xhrFields: {
                withCredentials: true
            },
            success: this.gameReady.bind(this),
            processData: false,
            contentType: 'application/json; charset=utf-8',
            data: JSON.stringify({token: gapi.auth.getToken().access_token,
                                  myid: gapi.hangout.getLocalParticipantId(),
                                  hangout: gapi.hangout.getHangoutId(),
                                 }),
        });
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
            var $player = $("<li/>");
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
            $('button.start-game').prop('disabled', false);
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

        var that = this;

        $.ajax({
            type: 'POST',
            url: serverPath + 'game/start',
            contentType: 'application/json; charset=utf-8',
            data: JSON.stringify({ players: this.participant_ids }),
            dataType: 'json',
            xhrFields: {
                withCredentials: true
            },
        }).done(this.handleGameState.bind(this))
            .fail(function() {that.ui.$start_button.prop('disabled', false)});
    };

    App.prototype.joinGame = function(gameid) {
        $.ajax({
            type: 'POST',
            url: serverPath + 'game/join',
            dataType: 'json',
            xhrFields: {
                withCredentials: true
            },
        }).done(this.handleGameState.bind(this));
    };

    App.prototype.gameStart = function(gameid) {
        console.log("gameStart(" + gameid + "), old == " + this.gameid)
        this.this_mission = null;
        this.this_proposal = null;
        this.leader = null;
        this.gameid = gameid;

        if ("$missionresults" in this.ui) {
            this.ui.$missionresults.empty();
        }

        this.revealRoles();
        $('.info-box').show();
        this.startInterval();
    };

    App.prototype.startMode = function() {
        this.ui.$start_button = $('button.start-game');
        this.ui.$gameplayers = $('div.start-mode ul.gameplayers');
        this.showGamePlayers();

        $('div.start-mode').show();
    };

    App.prototype.resetPickMode = function() {
        this.ui.$pickbox.hide();
        this.ui.$commitproposal.prop('disabled', false);
        this.ui.$pick.empty();
    };

    App.prototype.pickMode = function() {
        this.ui.$count = $('div.pick-mode span.playercount');

        this.ui.$pickbox = $('div.pick-mode form.proposal');
        this.ui.$pick = this.ui.$pickbox.children('ul.proposal');
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
        this.ui.$missionplayers = $('div.voting-mode ul.missionplayers');

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
        this.ui.$missionplayers = $('div.mission-mode ul.missionplayers');

        this.ui.$pickbox = $('div.mission-mode form.mission');
        this.ui.$success = this.ui.$pickbox.children('input.success');
        this.ui.$failure = this.ui.$pickbox.children('input.fail');
        this.ui.$commitmission = this.ui.$pickbox.children('button');

        this.resetMode = this.resetMissionMode;
        this.resetMode();

        $('div.mission-mode').show();
    };

    App.prototype.resetGameoverMode = function() {
        this.ui.$playercards.empty();
        $('button.start-game').prop('disabled', false);
    };

    App.prototype.gameoverMode = function() {
        this.ui.$result = $('div.gameover-mode span.result');
        this.ui.$playercards = $('div.gameover-mode ul.playercards');
        this.ui.$start_button = $('button.start-game');
        this.ui.$gameplayers = $('div.gameover-mode ul.gameplayers');

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

        this.ui.$tableplayers = $('div.table-order ol.players');
        this.ui.$leader = $('div.game-status span.leader');
        this.ui.$thismission = $('div.game-status span.thismission');
        this.ui.$thisproposal = $('div.game-status span.thisproposal');
        this.ui.$missionresults = $('div.mission-results ol.missions');
    }

    App.prototype.commitProposal = function() {
        if (this.gamestate != 'picking' || this.leader != this.mypos) {
            return false;
        }

        console.log("commitProposal");
        var $selected = $('input:checked', '#proposal');
        if ($selected.length != this.missionsize) {
            return false;
        }

        var that = this;

        var players = [];
        $selected.each(function(i) { players.push( $(this).val() ) });

        this.ui.$commitproposal.prop('disabled', true);
        this.ui.$pick.children('input').prop('disabled', true);

        $.ajax({
            type: 'POST',
            url: serverPath + 'game/propose',
            contentType: 'application/json; charset=utf-8',
            data: JSON.stringify({ players: players }),
            dataType: 'json',
            xhrFields: {
                withCredentials: true
            },
        }).done(this.handleGameState.bind(this))
            .fail(function() {
                that.ui.$commitproposal.prop('disabled', false)
                this.ui.$pick.children('input').prop('disabled', false);
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

        $.ajax({
            type: 'POST',
            url: serverPath + 'game/vote',
            contentType: 'application/json; charset=utf-8',
            data: JSON.stringify({ vote: vote }),
            dataType: 'json',
            xhrFields: {
                withCredentials: true
            },
        }).done(this.handleGameState.bind(this))
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

        $.ajax({
            type: 'POST',
            url: serverPath + 'game/mission',
            contentType: 'application/json; charset=utf-8',
            data: JSON.stringify({ action: $selected.val() }),
            dataType: 'json',
            xhrFields: {
                withCredentials: true
            },
        }).done(this.handleGameState.bind(this))
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
            var $li = $("<li/>");
            var $label = $("<label/>");
            var $input = $("<input type='checkbox'/>");
            $input.attr('value', id);
            $li.append($label);
            $label.text(this.playerName(i))
            $label.prepend($input);
            this.ui.$pick.append($li);
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
    };

    App.prototype.revealRoles = function() {
        $.ajax({
            type: 'POST',
            url: serverPath + 'game/reveal',
            dataType: 'json',
            xhrFields: {
                withCredentials: true
            },
        }).done(this.handleReveal.bind(this));
    };

    App.prototype.handleReveal = function(msg) {
        this.$msgbox.empty();
        this.$msgbox.dialog('option', 'title', 'You can see...');

        for (var i = 0; i < msg.length; i++) {
            var $box = $("<div class='reveal'/>");
            $box.text(msg[i].label);
            var $list = $("<ul class='players'/>");
            this.renderPlayers(msg[i].players, {}, $list);
            $box.append($list);
            this.$msgbox.append($box);
        }

        this.$msgbox.dialog("open");
    };

    App.prototype.fetchGameState = function() {
        if (this.fetchajax !== null) {
            this.fetchajax.abort();
        }
        this.fetchajax = $.ajax({
            type: 'POST',
            url: serverPath + 'game/state',
            dataType: 'json',
            xhrFields: {
                withCredentials: true
            },
        }).done(this.handleGameState.bind(this));
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

    App.prototype.renderPlayers = function (players, data, $list) {
        $list.empty();

        var includesme = false;

        for (var i = 0; i < players.length; i++) {
            var $player = $("<li/>");
            var text = this.playerNameById(players[i]);
            if (i in data) {
                text += ': ' + data[i];
            }
            $player.text(text);
            $list.append($player);
            if (players[i] == this.myid) {
                includesme = true;
            }
        }

        return includesme;
    };

    App.prototype.renderMissions = function (results) {
        for (var i = this.ui.$missionresults.children().length; i < results.length; i++) {
            var $li = $("<li/>");
            var text;
            if (results[i].fails > results[i].fails_allowed) {
                text = "Failed"
            }
            else {
                text = "Success"
            }
            if (results[i].fails > 0) {
                text += " (" + results[i].fails + " fails)"
            }
            $li.text(text);
            var players = results[i].players.map(this.playerName.bind(this));
            $li.attr('title', players.join(", "));
            $li.tooltip();
            this.ui.$missionresults.append($li);
        }
    };

    App.prototype.handleGameState = function (msg) {
        console.log(msg);
        this.fetchajax = null;
        this.players = msg.general.players;
        this.mypos = this.players.indexOf(this.myid);

        if (this.gameid != msg.general.gameid) {
            console.log("/state said we need to change games");
            this.gameStart(msg.general.gameid);
            gapi.hangout.data.submitDelta({gameid: msg.general.gameid});
        }

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
        if (msg.general.last_votes !== null) {
            for (var i = 0; i < msg.general.last_votes.length; i++) {
                if (msg.general.last_votes[i]) {
                    lastvote[i] = "approve";
                }
                else {
                    lastvote[i] = "reject";
                }
            }
        }
        this.renderPlayers(msg.general.players, lastvote, this.ui.$tableplayers);

        this.ui.$leader.text(this.playerName(msg.general.leader));
        this.ui.$thismission.text(msg.general.this_mission);
        this.ui.$thisproposal.text(msg.general.this_proposal);

        this.renderMissions(msg.general.mission_results);

        if (msg.general.state == 'picking') {
            if (msg.general.leader == this.mypos && this.leader != this.mypos) {
                this.becomeLeader();
            }
            this.leader = msg.general.leader;

            this.missionsize = msg.mission_size;

            this.ui.$count.text(msg.mission_size);
        }
        else if (msg.general.state == 'voting') {
            this.renderPlayers(msg.mission_players, {}, this.ui.$missionplayers);
        }
        else if (msg.general.state == 'mission') {
            var mymission = this.renderPlayers(msg.mission_players, {}, this.ui.$missionplayers);

            if (mymission) {
                this.ui.$success.prop('disabled', !msg.allow_success);
                this.ui.$failure.prop('disabled', !msg.allow_failure);
                this.ui.$pickbox.show();
            }
        }
        else if (msg.general.state == 'gameover') {
            this.ui.$result.text(msg.result);

            this.renderPlayers(msg.general.players, msg.cards, this.ui.$playercards);
            this.stopInterval();
        }
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

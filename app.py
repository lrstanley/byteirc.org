#!/usr/bin/python
import flask
import os
import xmlrpclib
import time
import re
from thread import start_new_thread as bg
from influxdb import InfluxDBClient



# load the configuration file
from conf import *

app = flask.Flask(__name__)
data = {}


@app.route('/')
@app.route('/<page>')
def main(page="index"):
    if os.path.isfile('templates/%s.html' % page):
        return flask.render_template(page + '.html', page=page, data=data)
    return flask.abort(404)


@app.route('/channel/<name>')
def channel(name="lobby"):
    return flask.render_template('chat.html', channel=name)


@app.route('/whois')
@app.route('/whois/<name>')
@app.route('/cwhois/<chan>')
def whois(name=False, chan=False):
    if not name and not chan:
        return flask.redirect('/')
    try:
        conn, key, user = irc_key(op=False)
        if name:
            cmd = conn.atheme.command(key, user, "*", "NickServ", "INFO", name).split("\n")[:-1]
        elif chan:
            cmd = conn.atheme.command(key, user, "*", "ChanServ", "INFO", "#" + chan).split("\n")[:-1]
        tmp = {}
        if name:
            tmp['account'] = re.search(r"\(account (.*?)\)", cmd[0]).groups()[0]
        elif chan:
            tmp['account'] = re.search(r"Information on (.*?)\:", cmd[0]).groups()[0]
        tmp['other'] = [[i.strip() for i in x.split(":", 1)] for x in cmd[1::]]
        if chan:
            chan_query = conn.atheme.command(key, user, "*", "ALIS", "LIST", "#" + chan).split('\n')
            # Attempt to pull topic, channel count, etc
            _tmp = [re.search(r"#?(.*?) +([0-9]+) :(.*)", a) for a in chan_query]
            if _tmp:
                _tmp = list(filter(None, _tmp)[0].groups())
                tmp['topic'] = _tmp[2]
                tmp['count'] = int(_tmp[1])
        return flask.render_template("whois.html", whois=tmp)
    except:
        return flask.render_template("whois.html", whois=False)


@app.errorhandler(404)
def page_not_found(error):
    return flask.render_template('404.html'), 404


@app.context_processor
def utility_processor():
    return dict(
        irc=data
    )


@app.after_request
def add_header(response):
    """
        Add headers to both force latest IE rendering engine or Chrome Frame,
        and also to cache the rendered page for 10 minutes.
    """
    response.headers['X-UA-Compatible'] = 'IE=Edge,chrome=1'
    # response.headers['Cache-Control'] = 'public, max-age=600'
    return response


def irc_key(op=False):
    user = xmlrpc_user_op if op else xmlrpc_user
    conn = xmlrpclib.ServerProxy("http://%s:%s/xmlrpc" % (xmlrpc_host, str(xmlrpc_port)))
    key = str(conn.atheme.login(user, xmlrpc_pass))
    return conn, key, user


def ircapi(service, command, op=False):
    try:
        conn, key, user = irc_key(op=op)
        cmd = conn.atheme.command(key, user, "*", service, command)
        return str(cmd)
    except:
        return False


@app.route('/data')
def query_data():
    return flask.jsonify(data)


@app.route('/test')
def testing():
    x = ircapi("BotServ", "BOTLIST", op=False).strip('\n').split('\n')[1:-2]
    bots = []
    for line in x:
        _tmp = {}
        # 1: Kilobyte (kilo@kilobyte.byteirc.org) [One thousand bytes]
        _tmp['id'], _tmp['nick'], _tmp['user'], _tmp['host'], _tmp['description'] = list(re.search(r"([0-9]+): (\S+) \((.*?)@(.*?)\) \[(.*?)\]", line).groups())
        bots.append(_tmp)
    return flask.jsonify(dict(data=bots))


influx = InfluxDBClient(influx_host, 8086, influx_user, influx_pass, influx_db)

def metrics(data):
    influx.write_points([{'measurement': 'stats', 'fields': {
        'accounts': data['accounts'],
        'nicks': data['nicks'],
        'channels': data['channels'],
        'active': data['active']
    }}])


def data_pull():
    global data
    while True:
        try:
            # Stats
            x = ircapi("OperServ", "UPTIME", op=True)
            data['accounts'] = int(re.search(r"Registered accounts: ([0-9]+)", x).groups()[0])
            data['nicks'] = int(re.search(r"Registered nicknames: ([0-9]+)", x).groups()[0])
            data['channels'] = int(re.search(r"Registered channels: ([0-9]+)", x).groups()[0])
            data['active'] = int(re.search(r"Users currently online: ([0-9]+)", x).groups()[0])

            metrics(data)

            # Notifications
            x = ircapi("InfoServ", "LIST", op=False).split('\n')
            notices = []
            for line in x:
                _tmp = re.search(r"([0-9]{1,}): \[.*?\] by (.*?) at ([0-9:]+) on ([0-9/]+): (.*)$", line)
                if _tmp:
                    _tmp = list(_tmp.groups())
                    notices.append({
                        'id': int(_tmp[0]),
                        'author': _tmp[1],
                        'time': _tmp[2],
                        'date': _tmp[3],
                        'message': _tmp[4]
                        })
            data['notifications'] = notices

            # Channels
            conn, key, user = irc_key(op=False)
            x = conn.atheme.command(key, user, "*", "ALIS", "LIST", "*", "-show", "t", "-min", "3", "-topic", "?").split('\n')
            channels = []
            for line in x:
                _tmp = re.search(r"#?(.*?) +([0-9]+) :(.*) \((.*)\)", line)
                if _tmp:
                    _tmp = list(_tmp.groups())
                    channels.append({
                        'name': _tmp[0],
                        'count': int(_tmp[1]),
                        'topic': _tmp[2],
                        'set_by': _tmp[3]
                        })
            data['channel_list'] = sorted(channels, key=lambda k: k['count'], reverse=True)

            # Bots
            x = ircapi("BotServ", "BOTLIST", op=False).strip('\n').split('\n')[1:-2]
            bots = []
            for line in x:
                _tmp = {}
                # 1: Kilobyte (kilo@kilobyte.byteirc.org) [One thousand bytes]
                _tmp['id'], _tmp['nick'], _tmp['user'], _tmp['host'], _tmp['description'] = list(re.search(r"([0-9]+): (\S+) \((.*?)@(.*?)\) \[(.*?)\]", line).groups())
                bots.append(_tmp)
            data['bots'] = bots
        except Exception as e:
            print("Exception in check: %s" % str(e))
        time.sleep(30)


bg(data_pull, ())

if __name__ == '__main__':
    app.debug = False
    app.run(host='0.0.0.0', port=8080)

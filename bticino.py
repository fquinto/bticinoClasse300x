import socket
import logging
import time
import hmac
import hashlib
import asyncio

LOGGER = logging.getLogger(__name__)
LOGGER.setLevel(logging.DEBUG)

# Acknowledge (OPEN message OK)
ACK="*#*1"
# Not-Acknowledge (OPEN message wrong)
NACK="*#*0"
# Monitor session
MONITOR="*99*1##"
# Commands session
#COMMANDS="*99*0##"
SPECIAL="*99*9##"


"""
verze fw:
-> *#*1##
<- *99*0##
-> *#184908184##
<- *#988765849##
-> *#*1##
<- *#1013**1##
-> *#1013**1*68*15*1*0##
-> *#*1##

http://www.pimyhome.org/wiki/index.php/OWN_OpenWebNet_Language_Reference

Standard	        *WHO*WHAT*WHERE##	                            Standard message
Status Request	    *#WHO*WHERE##	                                Request a state (e.g. if a light is ON or OFF)
Dimension Request	*#WHO*WHERE*DIMENSION##	                        Request dimension value
Dimension Write	    *#WHO*WHERE*#DIMENSION*VAL1*VAL2*...*VALn##	    Write dimension value(s)


WHO value	Function
0	        Scenario
1	        Lighting
2	        Automation
3	        Load control
4	        Temperature Control/Heating
5	        Burglar Alarm/Intrusion
6	        Door Entry System
7	        Video Door Entry System/multimedia
9	        Auxiliary
13	        Gateway/interfaces management
14	        Light+shutters actuators lock
15	        CEN/Scenario Scheduler, switch
16	        Sound System/Audio
17	        Scenario programming
18	        Energy Management
24	        Lighting Management
25	        CEN/Scenario Scheduler, buttons
1000	    Diagnostic
1001	    Automation diagnostic
1004	    Thermoregulation diagnostic failures
1013	    Device diagnostic

WHO=8 - Locks
ex: *8*19*20##      19=?start?  ; 20 = type 2 + addr 0
    *8*20*20##      20=?stop?

WHO=1013 - Device Diagnostic
DIMENSION	Description	        R/W	Syntax
1	        Device Type	        R	Get Device Type: *#1013**1##
2	        Firmware version	R	Get firmware version: *#1013**2##
3	        Hardware version	R	Get hardware version: *#1013**3##
6	        Micro version	    R	Get micro version: *#1013**6##
7	        ?	?	?
30	        ?	?	?

two modes: (send as first data)
COMMAND = *99*0##          // only for sending commands
MONITOR = *99*1##          // only for monitoring (when send command, then disconnects)

classe300X:
linphone:  /com/legrandgroup/c300x/linphone/VctLinphoneService.java
*8*19*4##  - fungovalo (ACK)
*8*20*4##

firmware update /com/legrandgroup/c300x/net/C1946a.java
*#130**1##   // status request
*130*1##
*130*2##

==answering machine==
disabled
s: *8*92##

enabled
s: *8*91##


==device diag:==
s: *#1013**1##
r: *#1013**1*68*15*1*0##

s: *#1013**2##
r: *#*1##
r: *#1013**2*1*1*41##

s: *#1013**3##
r: *#1013**3*0*0*0##


==dev info==
s: *#13**0##        // now ?
r: *#13**0*23*39*09*999##

s: *#13**1##        // date
r: *#13**1*04*10*05*2018##

s: *#13**10##       // ip address
r: *#13**10*192*168*111*4##

s: *#13**11##       // netmask
r: *#13**11*255*255*255*0##

s: *#13**15##
r: *#13**15*200##   // device - 200 = F454 (new?)

==ftp cmd===
*#13**#31*0##       // start ftp cfg
*#13**#33*0##       // stop ftp
*#13**#34*0##       // start ftp - another dir (empty)


==events:==
*7*73#1#100*##      // aktivace c300x displaye
*7*73#1#10*##       // zhasnutí displaye
*#8**33*0##         // zvonek vypnut
*#8**33*1##         // zvonek zapnut
*8*92##             // answ disabled
*8*91##             // answ enabled

====TODO==
s: *13*35*##

locks:
s: *8*19* device dev + device addr ##
s: *8*20* device dev + device addr ##

světla:
s: *8*21* device dev + device addr ##
s: *8*22* device dev + device addr ##

database
plantDbIdx,cid  ,id     ,uii ,description     ,name           ,icon,icon_color,deviceAddr,deviceDev,type,tecnology
5         ,10060,'10050',96  ,'Hlavni vchod'  ,'Hlavni vchod' ,''  ,''        ,'0'       ,'2'      ,'Lock','','','');
5         ,10060,'10050',110 ,'Vnitrni dvere' ,'Vnitrni dvere',''  ,''        ,'1'       ,'2'      ,'Lock','','','');
5         ,10060,'10050',123 ,'Rozvadec'      ,'Rozvadec'     ,''  ,''        ,'3'       ,'2'      ,'Lock','','','');

===zjistit===
s: *98*2##
s: *99*7*%s##
s: *13*00*%d            // sending id?
s: *13*28*##            // ?frame příjem?
s: *13*86*%d##          // logs?
%d*8*1*0
s: *#13**#0*%02d*%02d*%02d*##
s: *#1013**6*%i%i%i%i##
s: *#1013**7*%i%i%i%i##
s: *#1013**11*%i%i%i%i
s: *#1004*%s*23#
s: *1000*1001*0##

*#13*48*       // syslog
"""

def rshift(val, bits):
    return (val >> bits) & ((1 << (32-bits))-1)

class ExNoInitAck(Exception):
    pass

class ExDisconnected(Exception):
    pass

class ExWrongToken(Exception):
    pass

class Bticino():
    def __init__(self, mode_command=MONITOR, host='localhost', port=20000):
        super().__init__()
        self.sock = None
        self.host = host
        self.port = port
        self.running = False
        self.password = 710299916
        self.mode_command = mode_command    # monitor or commands
        self.reader = None
        self.writer = None

    async def close(self):
       if self.writer:
            self.writer.close()
            await self.writer.wait_closed()
            LOGGER.info("closed connection to %s", self.host)

    async def send(self, data):
        LOGGER.debug("sending: '%s'", data)
        self.writer.write(data.encode())
        await self.writer.drain()

    async def run(self):
        LOGGER.info("connecting to '%s', port=%d", self.host, self.port)
        self.reader, self.writer = await asyncio.open_connection(self.host, self.port)
        LOGGER.info("Starting bticino session '%s'", self.mode_command)

        self.running = True
        try:
            await self.runPrepare()

            while self.running:
                await self.oneLoop()
        except:
            LOGGER.exception("during run:")
            raise
        finally:
            await self.close()
        LOGGER.info("exiting session %s", self.mode_command)

    async def runPrepare(self):
        state = 'init'

        while True:
            frame = await self.reader.read(150)
            if not frame:
                raise ExDisconnected()

            frame = frame.decode()
            LOGGER.debug("rec frame: %s", frame)
            frames = frame.split('##')
            frame = frames[0]

            if state == 'init':
                if frame != ACK:
                    raise ExNoInitAck("no ACK on connect, instead: '%s'", frame)
                state = 'unauth'
                await self.send( self.mode_command)
                continue
            if state == 'unauth':
                # *#676132937##
                if frame == "*98*2":
                    await self.send("*#*1##")
                    state='auth_hmac'
                elif frame == ACK:
                    state = 'auth_ack'
                    LOGGER.debug("already authorized")
                    return
                else:
                    ans = self.answerChallenge(frame, self.password)
                    await self.send('*#' + ans + '##')
                    state = 'auth_ack'
                continue
            if state == 'auth_hmac':
                ans = self.hmacChallenge(frame, self.password)
                await self.send('*#' + ans[0] + '*' + ans[1] + '##')
                state = 'auth_resp'
                continue
            if state == 'auth_resp':
                await self.send("*#*1##")
                state = 'auth_ack'
                return

            if frame != ACK:
                raise ExWrongToken("no ACK on auth - wrong token, instead: '%s'", frame)
            return

    @staticmethod
    def nums2hex(indata):
        hexstr = ""
        for (n1, n2) in zip(indata[0::2], indata[1::2]):
            hexstr += "%1x" % int(n1+n2)
        return hexstr

    @staticmethod
    def hexstr2nums(hexstr):
        """
        in: 2A
        out: 0210
        """

        numstr = ""
        for hexnum in hexstr:
            numstr += "%02d" % int(hexnum, 16)
        return numstr

    def hmacChallenge(self, frame, password, randnumsdigest=None):
        LOGGER.debug("frame to challenge: %s", frame)

        password = str(password)

        inhex = self.nums2hex(frame[2:])

        m = hashlib.sha256()
        m.update(password.encode())
        hex_pwd_digest = m.hexdigest()

        if randnumsdigest is None:
            m = hashlib.sha256()
            m.update(b"xx")          # "random" digest
            hex_rnd_digest = m.hexdigest()
            random_nums = self.hexstr2nums(hex_rnd_digest)
        else:
            hex_rnd_digest = self.nums2hex(randnumsdigest)
            random_nums = randnumsdigest
            LOGGER.debug("use predefined rand digest: %s", random_nums)

        uuid = inhex + hex_rnd_digest + "736F70653E" + "636F70653E" + hex_pwd_digest

        m = hashlib.sha256()
        m.update(uuid.encode())
        hex_uuid_digest = m.hexdigest()
        num_uuid_digest = self.hexstr2nums(hex_uuid_digest)

        tmp = [random_nums, num_uuid_digest]
        LOGGER.debug("challenge: %s", tmp)
        return tmp

    def answerChallenge(self, frame, password):
        #  '#*' + x( challenge[2:]  )
        challenge = frame[2:]
#        LOGGER.debug("challenge='%s'", challenge)

        j = 0
        j3 = password
        j2 = j3

        for c in challenge:
            j &= 0xFFFFFFFF
            j2 &= 0xFFFFFFFF

            if c == '1':
                j2 = (j2 << 25) + rshift(j2,7)
            elif c == '2':
                j2 = (j2 << 28) + rshift(j2,4)
            elif c == '3':
                j2 = (j2 << 29) + rshift(j2,3)
            elif c == '4':
                j2 = rshift(j2,31) + (j2 << 1);
            elif c == '5':
                j2 = rshift(j2,27) + (j2 << 5);
            elif c == '6':
                j2 = rshift(j2,20) + (j2 << 12);
            elif c == '7':
                j2 = rshift((j2 & 4278190080),8) + (((65280 & j2) + ((255 & j2) << 24)) + rshift(16711680 & j2, 16))
            elif c == '8':
                j2 = rshift((j2 & 16711680),8) + (((65535 & j2) << 16) + rshift(j2,24))
            elif c == '9':
                j2 ^= -1
            j = j2
#            LOGGER.debug("char=%s; j2=%d", c, j2)

        j &= 0xFFFFFFFF
#        LOGGER.debug("answer=%s", str(j))
        return str(j)

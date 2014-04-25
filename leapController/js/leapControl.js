$(document).ready(function() {
    var ws;
    var controller = new Leap.Controller();
    var pointable;
    var stabilizedPosition = [0, 0, 0];

    var wsHost = "10.0.0.32"
    var wsPort = 3000
    var wsEP = "ws"

    var max_lat_pos = 150;
    var max_height = 350;
    var min_height = 150;

    var ticks = $('ul.ticks li');
    var len = ticks.length;
    var rotate = 0;

    var max_angle = 45;
    var max_speed = 99;
    var min_speed = 20;

    var degree = 0;
    var pointableLongPos = 0;
    var pointableLatPos = 0;

    var mappedSpeed = 0;
    var mappedAngle = 0;
    var velocityString = mappedSpeed + "," + mappedAngle

    for (i = 0; i < len; i++) {
        $(ticks[i]).css({
            '-webkit-transform': 'translateX(50%) rotate(' + rotate + 'deg) translateX(-50%)'
        });
        rotate += 10;
    }

    setInterval(function() {
        $(".speed-info")[0].innerText = ('000' + mappedSpeed).substr(-3)
        $('.meter .needle').css({
            '-webkit-transform': 'rotate(' + degree + 'deg)  translateZ(0) translate3d(0,0,0)'
        });
    }, 200);

    setInterval(function() {
        if (ws.readyState === 1) {
            var vel = mappedSpeed + "," + mappedAngle
            console.log("sending velocityString: " + vel);
            ws.send(vel);
        }
    }, 50);



    updateMappedAngle = function() {
        var absVal = Math.abs(pointableLatPos);
        if (pointableLatPos < 0) {
            if (absVal > max_lat_pos) {
                mappedAngle = (-1 * max_angle);
                return;
            }
            mappedAngle = -1 * Math.floor(map(absVal, 0, max_lat_pos, 0, max_angle))
        } else {
            if (absVal > max_lat_pos) {
                mappedAngle = max_angle;
                return;
            }
            mappedAngle = Math.floor(map(absVal, 0, max_lat_pos, 0, max_angle))
        }
    }

    updateMappedSpeed = function() {
        if (pointableLongPos < min_height) {
            mappedSpeed = 0;
            return;
        }
        if (pointableLongPos > max_height) {
            mappedSpeed = max_speed;
            return;
        }
        mappedSpeed = Math.floor(map(pointableLongPos, min_height, max_height, min_speed, max_speed))
    }

    mapHeightToAngle = function() {
        if (pointableLongPos < min_height) {
            degree = 0;
            return;
        }
        if (pointableLongPos > max_height) {
            degree = 180;
            return;
        }
        degree = Math.floor(map(pointableLongPos, min_height, max_height, 0, 180))
    }


    if (window.WebSocket) {
        ws = new WebSocket('ws://' + wsHost + ':' + wsPort + '/ws')
    }

    controller.on('connect', function() {
        setInterval(function() {
            var frame = controller.frame();
            if (frame.pointables.length > 0) {
                pointable = frame.pointables[0];
                stabilizedPosition = pointable.stabilizedTipPosition;
                pointableLongPos = Math.floor(stabilizedPosition[1]);
                pointableLatPos = Math.floor(stabilizedPosition[0]);
            } else {
                pointableLongPos = 0
                pointableLatPos = 0
            }
            mapHeightToAngle();
            updateMappedAngle();
            updateMappedSpeed();
        }, 200);
    });

    controller.connect();

    map = function(x, inmin, inmax, outmin, outmax) {
        return (x - inmin) * (outmax - outmin) / (inmax - inmin) + outmin
    }
})
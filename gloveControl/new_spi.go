package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"
	"unsafe"

	"code.google.com/p/go.net/websocket"

	"github.com/golang/glog"
)

const (
	SPI_CPHA = 0x01
	SPI_CPOL = 0x02

	SPI_MODE_0 = (0 | 0)
	SPI_MODE_1 = (0 | SPI_CPHA)
	SPI_MODE_2 = (SPI_CPOL | 0)
	SPI_MODE_3 = (SPI_CPOL | SPI_CPHA)

	SPI_IOC_WR_MODE          = 0x40016B01
	SPI_IOC_WR_BITS_PER_WORD = 0x40016B03
	SPI_IOC_WR_MAX_SPEED_HZ  = 0x40046B04

	SPI_IOC_RD_MODE          = 0x80016B01
	SPI_IOC_RD_BITS_PER_WORD = 0x80016B03
	SPI_IOC_RD_MAX_SPEED_HZ  = 0x80046B04

	SPI_IOC_MESSAGE_0   = 1073769216 //0x40006B00
	SPI_IOC_INCREMENTER = 2097152    //0x200000

	DEFAULT_SPEED_MAX = 5000000
	DEFAULT_BPW       = 8
)

var (
	wsHost = flag.String("wsHost", "localhost", "i2c bus to use")
	wsPort = flag.String("wsPort", "3000", "safe distance to stop the car")
	wsEP   = flag.String("wsEP", "ws", "width of the captured camera image")

	sensorMax = 0
	sensorMin = 0

	maxSpeed     = int64(99)
	comfortLevel = int64(20)
)

type SpiBus interface {
	TransferAndRecieveByteData([]uint8) (int, error)
}

type spiIocTransfer struct {
	tx_buf uint64
	rx_buf uint64

	length        uint32
	speed_hz      uint32
	delay_usecs   uint16
	bits_per_word uint8
}

type spiBus struct {
	file *os.File
	mode byte

	spiTransferData spiIocTransfer
}

func spi_ioc_message_n(n uint32) uint32 {
	return (SPI_IOC_MESSAGE_0 + (n * SPI_IOC_INCREMENTER))
}

func NewSpiBus(channel, maxSpeed, bitsPerWord int) (SpiBus, error) {
	var b *spiBus
	var err error
	b = new(spiBus)
	var data spiIocTransfer

	glog.V(2).Infof("spi: opening spi device for channel %v", channel)
	b.file, err = os.OpenFile(fmt.Sprintf("/dev/spidev0.%v", channel), os.O_EXCL, os.ModeExclusive)
	if err != nil {
		glog.V(3).Infof("spi: failed to open /dev/spidev0.%v due to %v", channel, err.Error())
		return nil, err
	}
	glog.V(3).Infof("spi: succesfully opened /dev/spidev0.%v", channel)

	var mode = uint8(0)
	glog.V(2).Infof("spi: setting spi mode to %v", mode)
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, b.file.Fd(), SPI_IOC_WR_MODE, uintptr(unsafe.Pointer(&mode)))
	if errno != 0 {
		err = syscall.Errno(errno)
		glog.V(3).Infof("spi: failed to set mode due to %v", err.Error())
		return nil, err
	}
	glog.V(3).Infof("spi: mode set to %v", mode)

	var speed_max uint32
	if maxSpeed > 0 {
		speed_max = uint32(maxSpeed)
	} else {
		speed_max = uint32(DEFAULT_SPEED_MAX)
	}

	glog.V(2).Infof("spi: setting spi speed_max to %v", speed_max)
	_, _, errno = syscall.Syscall(syscall.SYS_IOCTL, b.file.Fd(), SPI_IOC_WR_MAX_SPEED_HZ, uintptr(unsafe.Pointer(&speed_max)))
	if errno != 0 {
		err = syscall.Errno(errno)
		glog.V(3).Infof("spi: failed to set speed_max due to %v", err.Error())
		return nil, err
	}
	glog.V(3).Infof("spi: speed_max set to %v", speed_max)
	data.speed_hz = speed_max

	var bpw uint32
	if bitsPerWord > 0 {
		bpw = uint32(bitsPerWord)
	} else {
		bpw = uint32(DEFAULT_BPW)
	}

	glog.V(2).Infof("spi: setting spi bpw to %v", bpw)
	_, _, errno = syscall.Syscall(syscall.SYS_IOCTL, b.file.Fd(), SPI_IOC_WR_BITS_PER_WORD, uintptr(unsafe.Pointer(&bpw)))
	if errno != 0 {
		err = syscall.Errno(errno)
		glog.V(3).Infof("spi: failed to set bpw due to %v", err.Error())
		return nil, err
	}
	glog.V(3).Infof("spi: bpw set to %v", bpw)
	data.bits_per_word = uint8(bpw)
	data.delay_usecs = 0

	b.spiTransferData = data
	return b, err
}

func (b *spiBus) TransferAndRecieveByteData(tx_data []uint8) (rx_data int, err error) {
	myLen := len(tx_data)
	data := b.spiTransferData

	data.length = uint32(myLen)
	data.tx_buf = uint64(uintptr(unsafe.Pointer(&tx_data[0])))
	data.rx_buf = uint64(uintptr(unsafe.Pointer(&tx_data[0])))

	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, b.file.Fd(), uintptr(spi_ioc_message_n(1)), uintptr(unsafe.Pointer(&data)))
	if errno != 0 {
		err = syscall.Errno(errno)
		glog.V(3).Infof("spi: failed to read due to %v", err.Error())
		return 0, nil
	}
	return int(data.tx_buf), nil
}

func main() {
	flag.Parse()
	spiBus, err := NewSpiBus(0, 1000000, 0)
	if err != nil {
		panic(err)
	}

	maxSensorVal, minSensorVal := setupSensor(spiBus)

	url := fmt.Sprintf("ws://%v:%v/%v", *wsHost, *wsPort, *wsEP)
	protocol := ""
	origin := fmt.Sprintf("http://%v:%v", *wsHost, *wsPort)
	glog.Infof("connecting to websocket at %v", url)

	ws, err := websocket.Dial(url, protocol, origin)
	if err != nil {
		panic(err)
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, os.Kill)
	defer signal.Stop(quit)

	timer := time.Tick(150 * time.Millisecond)

	for {
		select {
		case <-timer:
			val, err := getSensorValue(spiBus)
			if err != nil {
				glog.Fatalf("spi: %v", err.Error())
			}

			mappedSpeed := Map(int64(minSensorVal+maxSensorVal-val), int64(minSensorVal), int64(maxSensorVal), 0, 99)
			if mappedSpeed > maxSpeed {
				mappedSpeed = maxSpeed
			}
			if mappedSpeed < comfortLevel {
				mappedSpeed = 0
			}
			velInfo := fmt.Sprintf("%v,0", mappedSpeed)
			if _, err := ws.Write([]byte(velInfo)); err != nil {
				panic(err)
			}
			fmt.Printf("mappedSpeed: %v\n", mappedSpeed)
		case <-quit:
			if _, err := ws.Write([]byte("0,0")); err != nil {
				panic(err)
			}
			return
		}
	}
}

func Map(x, inmin, inmax, outmin, outmax int64) int64 {
	return (x-inmin)*(outmax-outmin)/(inmax-inmin) + outmin
}

func getSensorValue(bus SpiBus) (uint16, error) {
	data := make([]uint8, 3)
	data[0] = 1
	data[1] = 128
	data[2] = 0
	var err error
	_, err = bus.TransferAndRecieveByteData(data)
	if err != nil {
		return uint16(0), err
	}
	return uint16(data[1]&0x03)<<8 | uint16(data[2]), nil
}

func setupSensor(bus SpiBus) (uint16, uint16) {
	glog.Infoln("********Setting up sensor********")
	time.Sleep(1 * time.Second)
	glog.Infoln("Calibrating rest position. Keep the glove in open position")

	ticker := time.Tick(1 * time.Second)
	quit := make(chan bool, 1)
	count := 3
	var initial = uint16(0)
	var final = uint16(0)
	var err error

first:
	for {
		select {
		case <-ticker:
			if count == 0 {
				quit <- true
			}
			initial, err = getSensorValue(bus)
			if err != nil {
				panic(err)
			}
			count = count - 1
		case <-quit:
			glog.Infof("Set open position to %v", initial)
			break first
		}
	}

	glog.Infoln("Calibrating close position. Keep the glove in closed position")
	ticker = time.Tick(1 * time.Second)
	count = 3

next:
	for {
		select {
		case <-ticker:
			if count == 0 {
				quit <- true
			}
			final, err = getSensorValue(bus)
			if err != nil {
				panic(err)
			}
			count = count - 1
		case <-quit:
			glog.Infof("Set closed position to %v", final)
			break next
		}
	}
	glog.Infoln("Good to go..!! Keep the glove in open position")
	time.Sleep(2 * time.Second)
	return initial, final
}

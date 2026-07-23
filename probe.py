"""芯片探测模块"""
import serial
import serial.tools.list_ports
import time

STM32_PID_MAP = {
    0x0413: "STM32F407xx", 0x0410: "STM32F40x/41x",
    0x0414: "STM32F411xE", 0x0420: "STM32F446xx",
    0x0440: "STM32L4x1", 0x0450: "STM32G0x1",
}

def list_serial_ports():
    ports = []
    for p in serial.tools.list_ports.comports():
        ports.append({
            "port": p.device, "name": p.description,
            "vid": p.vid or 0, "pid": p.pid or 0,
            "serial_number": p.serial_number or "",
        })
    return ports

def probe_stm32(port_name):
    try:
        ser = serial.Serial(port_name, 115200, timeout=1, parity=serial.PARITY_EVEN)
        ser.write(b'\x7F')
        if ser.read(1) != b'\x79':
            ser.close()
            return None
        ser.write(b'\x02\xFD')
        ser.read(1)
        data = ser.read(3)
        ser.close()
        if len(data) >= 2:
            pid = (data[1] << 8) | data[2]
            model = STM32_PID_MAP.get(pid, f"STM32 (PID: 0x{pid:04X})")
            return {"chip": model, "source": "stm32-bootloader", "confidence": 0.95}
    except:
        pass
    return None

def probe_esp32(port_name):
    try:
        ser = serial.Serial(port_name, 115200, timeout=1)
        ser.write(b'\x07\x07\x12 UUUUUUUUUUUUUUUUUUUUUUUUUUUUUUUU')
        time.sleep(0.15)
        if ser.read(2) == b'\x01\x02':
            ser.close()
            return {"chip": "ESP32/ESP8266", "source": "esptool", "confidence": 0.9}
        ser.close()
    except:
        pass
    return None

def quick_probe():
    results = []
    for port in list_serial_ports():
        r = {"port": port["port"], "name": port["name"], "vid": port["vid"], "pid": port["pid"]}
        chip = probe_stm32(port["port"]) or probe_esp32(port["port"])
        if chip:
            r.update(chip)
        else:
            r["chip"] = None
            r["needs_probe"] = True
        results.append(r)
    return results

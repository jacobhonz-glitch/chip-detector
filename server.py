"""芯片识别 Web 服务"""
from flask import Flask, jsonify, request, send_from_directory
from flask_cors import CORS
import probe
import webbrowser
import threading

app = Flask(__name__, static_folder='web', static_url_path='')
CORS(app)

CHIP_DB = {
    "STM32F407": {"sdk": "STM32CubeF4", "core": "Cortex-M4", "flash": "1MB", "ram": "192KB",
        "datasheet": "https://www.st.com/resource/en/datasheet/stm32f407vg.pdf"},
    "ESP32": {"sdk": "ESP-IDF", "core": "Xtensa LX6", "flash": "4MB", "ram": "520KB",
        "datasheet": "https://www.espressif.com/sites/default/files/documentation/esp32_datasheet_en.pdf"},
}

@app.route('/')
def index():
    return send_from_directory('web', 'index.html')

@app.route('/api/health')
def health():
    return jsonify({"success": True, "data": {"status": "ok", "version": "0.3.0"}})

@app.route('/api/scan')
def scan():
    ports = probe.list_serial_ports()
    return jsonify({"success": True, "data": {"serial_ports": ports, "total": len(ports)}})

@app.route('/api/quick-detect', methods=['POST'])
def quick_detect():
    results = probe.quick_probe()
    return jsonify({"success": True, "data": {"devices": results, "total": len(results)}})

@app.route('/api/chip/report', methods=['POST'])
def chip_report():
    chip = request.json.get('chip', '')
    print(f"📡 芯片上报: {chip}")
    for k, v in CHIP_DB.items():
        if k.lower() in chip.lower():
            return jsonify({"success": True, "data": v})
    return jsonify({"success": True, "data": {"sdk": "请查看数据手册", "core": "未知"}})

if __name__ == '__main__':
    threading.Timer(1.5, lambda: webbrowser.open('http://localhost:8080')).start()
    print("Chip Detector v0.3 - http://localhost:8080")
    app.run(host='0.0.0.0', port=8080, debug=False)

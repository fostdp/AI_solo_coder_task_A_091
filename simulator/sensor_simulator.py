#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
坎儿井传感器模拟器
模拟每条暗渠每1小时通过传感器上报流量、水位、竖井水位、蒸发量等数据
支持通过HTTP API和MQTT两种方式上报
"""

import json
import time
import random
import math
import requests
import paho.mqtt.client as mqtt
from datetime import datetime, timedelta
from typing import Dict, List, Optional

class KarezSensorSimulator:
    def __init__(self, 
                 karez_id: int = 1,
                 api_base_url: str = "http://localhost:8080/api",
                 mqtt_broker: Optional[str] = None,
                 mqtt_port: int = 1883,
                 mqtt_topic_prefix: str = "karez",
                 use_mqtt: bool = False,
                 interval_seconds: int = 3600):
        
        self.karez_id = karez_id
        self.api_base_url = api_base_url
        self.mqtt_broker = mqtt_broker
        self.mqtt_port = mqtt_port
        self.mqtt_topic_prefix = mqtt_topic_prefix
        self.use_mqtt = use_mqtt
        self.interval_seconds = interval_seconds
        
        self.segments = []
        self.shafts = []
        self.branches = []
        
        self.mqtt_client = None
        
        self.base_flow = 0.08
        self.current_flow = self.base_flow
        
        self.load_karez_config()
        
        if use_mqtt and mqtt_broker:
            self.setup_mqtt()
    
    def load_karez_config(self):
        """加载坎儿井配置"""
        try:
            resp = requests.get(f"{self.api_base_url}/karez/{self.karez_id}/segments", timeout=5)
            if resp.status_code == 200:
                self.segments = resp.json()
            
            resp = requests.get(f"{self.api_base_url}/karez/{self.karez_id}/shafts", timeout=5)
            if resp.status_code == 200:
                self.shafts = resp.json()
            
            resp = requests.get(f"{self.api_base_url}/karez/{self.karez_id}/branches", timeout=5)
            if resp.status_code == 200:
                self.branches = resp.json()
        except Exception as e:
            print(f"Warning: Failed to load karez config from API: {e}")
            print("Using default configuration...")
            self._use_default_config()
    
    def _use_default_config(self):
        """使用默认配置"""
        self.segments = [
            {"id": 1, "segment_name": "首部暗渠段", "segment_order": 1, "length": 800, "width": 0.8, "height": 1.2, "slope": 0.00625},
            {"id": 2, "segment_name": "中部暗渠段", "segment_order": 2, "length": 1800, "width": 0.8, "height": 1.2, "slope": 0.00556},
            {"id": 3, "segment_name": "尾部暗渠段", "segment_order": 3, "length": 1600, "width": 0.8, "height": 1.2, "slope": 0.00938},
            {"id": 4, "segment_name": "龙口段", "segment_order": 4, "length": 1000, "width": 1.0, "height": 1.5, "slope": 0.06},
        ]
        self.shafts = [
            {"id": i, "shaft_name": f"竖井-{i}", "shaft_order": i, "shaft_depth": 120 + i*2, "diameter": 0.8}
            for i in range(1, 21)
        ]
        self.branches = [
            {"id": 1, "branch_name": "东支渠", "max_flow": 0.08},
            {"id": 2, "branch_name": "西支渠", "max_flow": 0.08},
            {"id": 3, "branch_name": "南支渠", "max_flow": 0.05},
        ]
    
    def setup_mqtt(self):
        """设置MQTT客户端"""
        try:
            self.mqtt_client = mqtt.Client(client_id=f"karez_simulator_{self.karez_id}")
            self.mqtt_client.connect(self.mqtt_broker, self.mqtt_port, 60)
            self.mqtt_client.loop_start()
            print(f"MQTT connected to {self.mqtt_broker}:{self.mqtt_port}")
        except Exception as e:
            print(f"Warning: Failed to connect MQTT: {e}")
            self.use_mqtt = False
    
    def generate_sensor_data(self, segment_id: int, sensor_type: str, current_time: datetime) -> Dict:
        """生成传感器数据"""
        hour = current_time.hour
        
        day_factor = 1.0 + 0.1 * math.sin(hour * math.pi / 12 - math.pi/2)
        
        random_noise = random.gauss(0, 0.02)
        
        if sensor_type == "flow":
            flow = self.base_flow * day_factor * (1 + random_noise)
            flow = max(0.01, min(0.15, flow))
            
            seg = next((s for s in self.segments if s["id"] == segment_id), None)
            if seg and seg.get("segment_order", 0) > 1:
                loss_factor = 0.98 + random.uniform(-0.01, 0.01)
                flow *= loss_factor
            
            return {
                "karez_id": self.karez_id,
                "segment_id": segment_id,
                "sensor_type": "flow",
                "sensor_id": f"flow_seg_{segment_id}",
                "flow_rate": round(flow, 6),
                "velocity": round(flow / 0.96, 6),
                "time": current_time.isoformat()
            }
        
        elif sensor_type == "water_level":
            water_depth = 0.5 + 0.2 * math.sin(hour * math.pi / 12) + random.uniform(-0.05, 0.05)
            water_depth = max(0.2, min(1.0, water_depth))
            
            return {
                "karez_id": self.karez_id,
                "segment_id": segment_id,
                "sensor_type": "water_level",
                "sensor_id": f"level_seg_{segment_id}",
                "water_level": round(water_depth, 4),
                "time": current_time.isoformat()
            }
        
        elif sensor_type == "evaporation":
            base_evap = 2.0
            temp_factor = 1.0 + 0.5 * math.sin(hour * math.pi / 12 - math.pi/2)
            evaporation = base_evap * temp_factor * (1 + random.uniform(-0.1, 0.1))
            
            return {
                "karez_id": self.karez_id,
                "segment_id": segment_id,
                "sensor_type": "evaporation",
                "sensor_id": f"evap_seg_{segment_id}",
                "evaporation": round(evaporation, 4),
                "temperature": round(25 + 10 * math.sin(hour * math.pi / 12 - math.pi/2), 2),
                "time": current_time.isoformat()
            }
        
        elif sensor_type == "turbidity":
            turbidity = 5.0 + 3.0 * random.random()
            return {
                "karez_id": self.karez_id,
                "segment_id": segment_id,
                "sensor_type": "turbidity",
                "sensor_id": f"turb_seg_{segment_id}",
                "turbidity": round(turbidity, 2),
                "time": current_time.isoformat()
            }
        
        return {}
    
    def generate_shaft_data(self, shaft_id: int, current_time: datetime) -> Dict:
        """生成竖井传感器数据"""
        hour = current_time.hour
        
        water_level = 80 + 10 * math.sin(hour * math.pi / 12) + random.uniform(-2, 2)
        
        return {
            "karez_id": self.karez_id,
            "shaft_id": shaft_id,
            "sensor_type": "shaft_level",
            "sensor_id": f"shaft_level_{shaft_id}",
            "shaft_water_level": round(water_level, 3),
            "time": current_time.isoformat()
        }
    
    def send_data_http(self, data: Dict) -> bool:
        """通过HTTP API发送数据"""
        try:
            resp = requests.post(f"{self.api_base_url}/sensor", json=data, timeout=5)
            if resp.status_code == 200:
                return True
            else:
                print(f"HTTP error: {resp.status_code} - {resp.text}")
                return False
        except Exception as e:
            print(f"HTTP send failed: {e}")
            return False
    
    def send_data_mqtt(self, data: Dict) -> bool:
        """通过MQTT发送数据"""
        if not self.mqtt_client:
            return False
        
        try:
            topic = f"{self.mqtt_topic_prefix}/sensor/{data['sensor_type']}/{data['sensor_id']}"
            payload = json.dumps(data)
            self.mqtt_client.publish(topic, payload, qos=1)
            return True
        except Exception as e:
            print(f"MQTT send failed: {e}")
            return False
    
    def send_data(self, data: Dict) -> bool:
        """发送数据（根据配置选择方式）"""
        if self.use_mqtt:
            return self.send_data_mqtt(data)
        else:
            return self.send_data_http(data)
    
    def run_cycle(self, current_time: Optional[datetime] = None):
        """运行一个模拟周期"""
        if current_time is None:
            current_time = datetime.now()
        
        print(f"\n[{current_time.isoformat()}] Starting sensor simulation cycle...")
        
        data_count = 0
        
        for seg in self.segments:
            seg_id = seg["id"]
            
            for sensor_type in ["flow", "water_level", "evaporation", "turbidity"]:
                data = self.generate_sensor_data(seg_id, sensor_type, current_time)
                if data:
                    if self.send_data(data):
                        data_count += 1
                    else:
                        print(f"  Failed to send {sensor_type} data for segment {seg_id}")
        
        for shaft in self.shafts[:10]:
            shaft_id = shaft["id"]
            data = self.generate_shaft_data(shaft_id, current_time)
            if data:
                if self.send_data(data):
                    data_count += 1
                else:
                    print(f"  Failed to send shaft data for shaft {shaft_id}")
        
        self.base_flow += random.uniform(-0.001, 0.001)
        self.base_flow = max(0.05, min(0.12, self.base_flow))
        
        print(f"  Sent {data_count} sensor readings")
        return data_count
    
    def run_continuous(self):
        """持续运行模拟器"""
        print(f"Starting continuous karez sensor simulator for karez #{self.karez_id}")
        print(f"  Mode: {'MQTT' if self.use_mqtt else 'HTTP'}")
        print(f"  Interval: {self.interval_seconds} seconds")
        print(f"  API URL: {self.api_base_url}")
        if self.use_mqtt:
            print(f"  MQTT Broker: {self.mqtt_broker}:{self.mqtt_port}")
        print("Press Ctrl+C to stop\n")
        
        try:
            while True:
                self.run_cycle()
                time.sleep(self.interval_seconds)
        except KeyboardInterrupt:
            print("\n\nSimulator stopped by user")
            if self.mqtt_client:
                self.mqtt_client.loop_stop()
                self.mqtt_client.disconnect()
    
    def run_backfill(self, days: int = 7):
        """回溯生成历史数据"""
        print(f"Backfilling {days} days of sensor data...")
        
        end_time = datetime.now()
        start_time = end_time - timedelta(days=days)
        
        total_readings = 0
        current = start_time
        
        while current <= end_time:
            count = self.run_cycle(current)
            total_readings += count
            current += timedelta(hours=1)
            
            if total_readings % 100 == 0:
                print(f"  Progress: {current.isoformat()} - {total_readings} readings")
        
        print(f"Backfill complete. Total readings: {total_readings}")


def main():
    import argparse
    
    parser = argparse.ArgumentParser(description="坎儿井传感器模拟器")
    parser.add_argument("--karez-id", type=int, default=1, help="坎儿井ID")
    parser.add_argument("--api-url", type=str, default="http://localhost:8080/api", help="API基础URL")
    parser.add_argument("--mqtt", action="store_true", help="使用MQTT发送数据")
    parser.add_argument("--mqtt-broker", type=str, default="localhost", help="MQTT Broker地址")
    parser.add_argument("--mqtt-port", type=int, default=1883, help="MQTT端口")
    parser.add_argument("--interval", type=int, default=3600, help="上报间隔（秒）")
    parser.add_argument("--backfill", type=int, default=0, help="回溯生成N天历史数据")
    parser.add_argument("--once", action="store_true", help="只运行一次")
    
    args = parser.parse_args()
    
    simulator = KarezSensorSimulator(
        karez_id=args.karez_id,
        api_base_url=args.api_url,
        mqtt_broker=args.mqtt_broker if args.mqtt else None,
        mqtt_port=args.mqtt_port,
        use_mqtt=args.mqtt,
        interval_seconds=args.interval
    )
    
    if args.backfill > 0:
        simulator.run_backfill(days=args.backfill)
    elif args.once:
        simulator.run_cycle()
    else:
        simulator.run_continuous()


if __name__ == "__main__":
    main()

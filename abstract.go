package main

// Representation of a network device's metadata (currently biased towards
// optical data).
type DeviceData struct {
	Host         string
	Error        string               `json:",omitempty"`
	OpticsByPort map[uint]*OpticsData `json:",omitempty"`
}

// Representation of a network device port's L3 and optical metrics.
type OpticsData struct {
	Speed uint64

	// TODO: connector present? admin? oper?
	// TODO: optical module vendor / model / serial

	InErrors        uint64
	InOctets        uint64
	InUnicastPkts   uint64
	InMulticastPkts uint64
	InBroadcastPkts uint64

	OutErrors        uint64
	OutOctets        uint64
	OutUnicastPkts   uint64
	OutMulticastPkts uint64
	OutBroadcastPkts uint64

	// Lane 0 is the whole module, others ones are actual lanes
	ModuleTemperature float32
	ModuleVoltage     float32
	LaneCount         uint32
	SensorsByLane     map[uint]*OpticalSensor
}

// Representation of an optical module's sensor data.
type OpticalSensor struct {
	LaserTemperature   float32 // Celsius
	RxLaserPower       float32 // dBm
	TxLaserBiasCurrent float32 // Amperes
	TxLaserPower       float32 // dBm
}

func (self *OpticalSensor) IsNonZero() bool {
	return (self.LaserTemperature > 0 || self.RxLaserPower > 0 ||
		self.TxLaserPower > 0 || self.TxLaserBiasCurrent > 0)
}

// Compiles a given MIB dataset into a summary DeviceData. May cross-reference
// entries between MIBs. May convert raw values into specific units/dimensions.
// Currently filters out direct-attach cables and breakout configurations.
func NewDeviceData(host string, mib *OpticsMIB) *DeviceData {
	opticsByID := make(map[uint]*OpticsData)
	opticsByPort := make(map[uint]*OpticsData)

	extractInterfaceData(mib, opticsByID, opticsByPort)
	extractInterfaceHCData(mib, opticsByPort)

	// TODO: detect vendor?
	extractAristaData(mib, opticsByPort)
	extractJuniperData(mib, opticsByID)

	// TODO: matching for EntityPhysical to get manufacturer / serial etc.
	//       (unfortunately only available on Arista devices…)
	validOpticsData := cleanupOpticsData(opticsByPort)
	return &DeviceData{
		Host:         host,
		OpticsByPort: validOpticsData,
	}
}

// Builds a DeviceData instance with an error message (no data).
func NewDeviceDataError(host string, error string) *DeviceData {
	return &DeviceData{
		Host:         host,
		Error:        error,
		OpticsByPort: nil,
	}
}

func extractInterfaceData(
	mib *OpticsMIB,
	opticsByID map[uint]*OpticsData,
	opticsByPort map[uint]*OpticsData,
) {
	for id, entry := range mib.Interface {
		port, ok := interfaceNameToPort(entry.Descr)
		if !ok {
			continue
		}

		intf, ok := opticsByPort[port]
		if !ok {
			intf = &OpticsData{
				SensorsByLane: make(map[uint]*OpticalSensor),
			}

			opticsByID[id] = intf
			opticsByPort[port] = intf
		}

		// Speed is specified in bits/sec, but modern systems use megabits/sec
		intf.Speed += uint64(entry.Speed) / 1000000

		intf.InErrors += uint64(entry.InErrors)
		intf.InOctets += uint64(entry.InOctets)
		intf.InUnicastPkts += uint64(entry.InUcastPkts)

		intf.OutErrors += uint64(entry.OutErrors)
		intf.OutOctets += uint64(entry.OutOctets)
		intf.OutUnicastPkts += uint64(entry.OutUcastPkts)
	}
}

func extractInterfaceHCData(mib *OpticsMIB, opticsByPort map[uint]*OpticsData) {
	// As we summarize values by port, we should reset existing values if the
	// device supports 64-bit counters.
	if len(mib.InterfaceHC) > 0 {
		for _, entry := range opticsByPort {
			entry.Speed = 0
			entry.InOctets = 0
			entry.InUnicastPkts = 0
			entry.OutOctets = 0
			entry.OutUnicastPkts = 0
		}
	}

	for _, entry := range mib.InterfaceHC {
		port, ok := interfaceNameToPort(entry.Name)
		if !ok {
			continue
		}

		intf := opticsByPort[port]

		intf.Speed += entry.HighSpeed

		intf.InOctets += entry.HCInOctets
		intf.InUnicastPkts += entry.HCInUcastPkts
		intf.InMulticastPkts += entry.HCInMulticastPkts
		intf.InBroadcastPkts += entry.HCInBroadcastPkts

		intf.OutOctets += entry.HCOutOctets
		intf.OutUnicastPkts += entry.HCOutUcastPkts
		intf.OutMulticastPkts += entry.HCOutMulticastPkts
		intf.OutBroadcastPkts += entry.HCOutBroadcastPkts
	}
}

func extractAristaData(mib *OpticsMIB, opticsByPort map[uint]*OpticsData) {
	const (
		ModuleTemperatureSensor = 1
		ModuleVoltageSensor     = 2
	)
	const (
		TxLaserBiasCurrentSensor = 1
		TxLaserPowerSensor       = 2
		RxLaserPowerSensor       = 3
	)

	for id, entry := range mib.Sensor {
		// OID format for DOM sensors on Arista is 1003PP2LS:
		//   PP: port number
		//   L:  lane number (0 = module)
		//   S:  sensor
		//       if L == 0: (1 = Module temperature, 2 = Module current)
		//       else: (1 = TX bias, 2 = TX power, 3 = RX power)
		if id/100000 != 1003 {
			continue
		}

		// See above comment for details.
		sub := id % 100000
		port := sub / 1000
		lane := (sub / 10) % 10
		sensorId := sub % 10

		intf := opticsByPort[port]

		// Lane 0 is for module sensors (as opposed to individual lanes)
		isModuleSensors := (lane == 0)

		if isModuleSensors {
			switch sensorId {
			case ModuleTemperatureSensor:
				intf.ModuleTemperature = entry.Float32()
			case ModuleVoltageSensor:
				intf.ModuleVoltage = entry.Float32()
			}
		} else {
			sensor, ok := intf.SensorsByLane[lane]
			if !ok {
				sensor = &OpticalSensor{}
				intf.SensorsByLane[lane] = sensor
			}

			// XXX: On PEs we have -1000000mW RX power on some down interfaces,
			//      which causes NaN values as we compute logarithms.
			//      We default to 1 because log(0) = -Inf.
			if entry.Value < 0 {
				entry.Value = 1
			}

			switch sensorId {
			case TxLaserBiasCurrentSensor:
				sensor.TxLaserBiasCurrent = entry.Float32()
			case TxLaserPowerSensor:
				sensor.TxLaserPower = wattsToDecibellMilliwatts(entry.Float32())
			case RxLaserPowerSensor:
				sensor.RxLaserPower = wattsToDecibellMilliwatts(entry.Float32())
			}
		}
	}
}

func extractJuniperData(mib *OpticsMIB, opticsByID map[uint]*OpticsData) {
	// Extract module sensor values.
	for id, entry := range mib.JuniperDOM {
		intf, ok := opticsByID[id]
		if !ok {
			continue
		}

		intf.ModuleTemperature = float32(entry.Temperature)
		intf.ModuleVoltage = float32(entry.Voltage) / 1000
		intf.LaneCount = uint32(entry.LaneCount)
	}

	// Extract lane sensor values.
	for lane, cont := range mib.JuniperLaneDOM {
		// Juniper lane numbering starts at 0 as module sensors are separate, but
		// our numbering starts at 1 for inter-device consistency.
		lane += 1

		for id, entry := range cont.Entries {
			intf, ok := opticsByID[id]
			if !ok {
				continue
			}

			sensor, ok := intf.SensorsByLane[lane]
			if !ok {
				sensor = &OpticalSensor{}
				intf.SensorsByLane[lane] = sensor
			}

			sensor.LaserTemperature = float32(entry.LaserTemperature)
			sensor.RxLaserPower = float32(entry.RxLaserPower) / 100
			sensor.TxLaserBiasCurrent = float32(entry.TxLaserBiasCurrent) / 1000000
			sensor.TxLaserPower = float32(entry.TxLaserPower) / 100
		}
	}
}

// Discards ports that have no sensors, and lane that have nil/zero sensor
// values. These are usually direct-attach cables or useless defaults.
func cleanupOpticsData(opticsByPort map[uint]*OpticsData) map[uint]*OpticsData {
	cleanData := make(map[uint]*OpticsData)
	for port, entry := range opticsByPort {
		// Discard entries with no lanes.
		if len(entry.SensorsByLane) == 0 {
			continue
		}

		// Keep data if there is at least one lane with non-nil measurements.
		for _, lane := range entry.SensorsByLane {
			if lane.IsNonZero() {
				cleanData[port] = entry
				break
			}
		}
	}

	return cleanData
}

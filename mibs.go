package main

import (
	"math"
)

// Describes the hierarchy of MIBs we need to obtain from hosts.
// Keys in maps are the value of the last component of the OID for array/maps.
// (See snmpmagic package for details)
type OpticsMIB struct {
	Interface   map[uint]*InterfaceEntry      `snmp:".1.3.6.1.2.1.2.2.1"`
	InterfaceHC map[uint]*InterfaceHCEntry    `snmp:".1.3.6.1.2.1.31.1.1.1"`
	Entity      map[uint]*EntityPhysicalEntry `snmp:".1.3.6.1.2.1.47.1.1.1.1"`
	Sensor      map[uint]*SensorEntry         `snmp:".1.3.6.1.2.1.99.1.1.1"`

	JuniperDOM     map[uint]*JuniperModuleDOMEntry     `snmp:".1.3.6.1.4.1.2636.3.60.1.1.1.1"`
	JuniperLaneDOM map[uint]*JuniperModuleLaneDOMEntry `snmp:".1.3.6.1.4.1.2636.3.60.1.2.1"`
}

type EntityPhysicalEntry struct {
	Descr        string `snmp:"2"`
	VendorType   string `snmp:"3"`
	ContainedIn  int32  `snmp:"4"`
	Class        int32  `snmp:"5"`
	ParentRelPos int32  `snmp:"6"`
	Name         string `snmp:"7"`
	HardwareRev  string `snmp:"8"`
	FirmwareRev  string `snmp:"9"`
	SoftwareRev  string `snmp:"10"`
	SerialNum    string `snmp:"11"`
	MfgName      string `snmp:"12"`
	ModelName    string `snmp:"13"`
	Alias        string `snmp:"14"`
	AssetID      string `snmp:"15"`
	IsFRU        bool   `snmp:"16"`
	MfgDate      string `snmp:"17"`
	Uris         string `snmp:"18"`
}

type SensorDataType int32

const (
	TypeOther SensorDataType = iota + 1
	TypeUnknown
	TypeVoltsAC
	TypeVoltsDC
	TypeAmperes
	TypeWatts
	TypeHertz
	TypeCelsius
	TypePercentRH
	TypeRPM
	TypeCMM
	TypeTruthvalue
)

type SensorDataScale int32

const (
	Yocto SensorDataScale = iota + 1
	Zepto
	Atto
	Femto
	Pico
	Nano
	Micro
	Milli
	Units
	Kilo
	Mega
	Giga
	Tera
	Exa
	Peta
	Zetta
	Yotta
)

type SensorEntry struct {
	Type            SensorDataType  `snmp:"1"`
	Scale           SensorDataScale `snmp:"2"`
	Precision       int32           `snmp:"3"`
	Value           int32           `snmp:"4"`
	OperStatus      int32           `snmp:"5"`
	UnitsDisplay    string          `snmp:"6"`
	ValueTimeStamp  uint32          `snmp:"7"`
	ValueUpdateRate uint32          `snmp:"8"`
}

func (self *SensorEntry) Float32() float32 {
	scalePower := (self.Scale - 9) * 3
	scaleFactor := math.Pow(10, float64(scalePower))
	return float32(float64(self.Value) * scaleFactor)
}

type InterfaceAdminStatus int32

const (
	AdminUp InterfaceAdminStatus = iota + 1
	AdminDown
	AdminTesting
)

type InterfaceOperStatus int32

const (
	OperUp InterfaceOperStatus = iota + 1
	OperDown
	OperTesting
	OperUnknown
	OperDormant
	OperNotPresent
	OperLowerLayerDown
)

type InterfaceEntry struct {
	Descr           string               `snmp:"2"`
	Type            int32                `snmp:"3"`
	Mtu             int32                `snmp:"4"`
	Speed           uint32               `snmp:"5"`
	PhysAddress     []byte               `snmp:"6"`
	AdminStatus     InterfaceAdminStatus `snmp:"7"`
	OperStatus      InterfaceOperStatus  `snmp:"8"`
	LastChange      uint32               `snmp:"9"`
	InOctets        uint32               `snmp:"10"`
	InUcastPkts     uint32               `snmp:"11"`
	InDiscards      uint32               `snmp:"13"`
	InErrors        uint32               `snmp:"14"`
	InUnknownProtos uint32               `snmp:"15"`
	OutOctets       uint32               `snmp:"16"`
	OutUcastPkts    uint32               `snmp:"17"`
	OutDiscards     uint32               `snmp:"19"`
	OutErrors       uint32               `snmp:"20"`
}

type InterfaceHCEntry struct {
	Name                     string `snmp:"1"`
	InMulticastPkts          uint32 `snmp:"2"`
	InBroadcastPkts          uint32 `snmp:"3"`
	OutMulticastPkts         uint32 `snmp:"4"`
	OutBroadcastPkts         uint32 `snmp:"5"`
	HCInOctets               uint64 `snmp:"6"`
	HCInUcastPkts            uint64 `snmp:"7"`
	HCInMulticastPkts        uint64 `snmp:"8"`
	HCInBroadcastPkts        uint64 `snmp:"9"`
	HCOutOctets              uint64 `snmp:"10"`
	HCOutUcastPkts           uint64 `snmp:"11"`
	HCOutMulticastPkts       uint64 `snmp:"12"`
	HCOutBroadcastPkts       uint64 `snmp:"13"`
	LinkUpDownTrapEnable     bool   `snmp:"14"`
	HighSpeed                uint64 `snmp:"15"`
	PromiscuousMode          bool   `snmp:"16"`
	ConnectorPresent         bool   `snmp:"17"`
	Alias                    string `snmp:"18"`
	CounterDiscontinuityTime uint64 `snmp:"19"`
}

type JuniperModuleDOMEntry struct {
	Temperature int32 `snmp:"8"`  // Celsius × 10^0
	Voltage     int32 `snmp:"25"` // Volts × 10^3
	LaneCount   int32 `snmp:"30"`
}

type JuniperModuleLaneDOMEntry struct {
	Entries map[uint]*JuniperLaneDOMEntry `snmp:"1"`
}

type JuniperLaneDOMEntry struct {
	RxLaserPower       int32 `snmp:"6"` // dBm × 10^2
	TxLaserBiasCurrent int32 `snmp:"7"` // Amperes × 10^-6
	TxLaserPower       int32 `snmp:"8"` // dBm × 10^2
	LaserTemperature   int32 `snmp:"9"` // Celsius × 10^0
}

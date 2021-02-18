package useragent

import "github.com/avct/uasurfer"

const (
	Unknown = 0
	Windows = 1
	MacOSX  = 2
	Linux   = 3
	Android = 4
	iOS     = 5
	Mobile  = 6
	Desktop = 7
	Tablet  = 8
	Others  = 9
	Bot     = 10
	User1   = 12 // user freq counter

	EndOfUA = 13
)

type UserAgents []int

func Parser(ua string) UserAgents {
	u := uasurfer.Parse(ua)
	switch u.OS.Name {
	case uasurfer.OSAndroid:
		return []int{Android, Mobile}
	case uasurfer.OSiOS:
		if u.OS.Platform == uasurfer.PlatformiPad {
			return []int{Android, Mobile, Tablet}
		}
		return []int{iOS, Mobile}
	case uasurfer.OSBot:
		return []int{Bot}
	case uasurfer.OSWindows:
		return []int{Windows, Desktop}
	case uasurfer.OSLinux:
		return []int{Linux, Desktop}
	case uasurfer.OSMacOSX:
		return []int{MacOSX, Desktop}
	default:
		return []int{Unknown}
	}
}

func (ua UserAgents) IsBot() bool {
	return len(ua) == 1 && ua[0] == Bot
}

func StringToEnum(in string) int {
	switch in {
	case "Windows":
		return Windows
	case "MacOSX":
		return MacOSX
	case "Linux":
		return Linux
	case "Android":
		return Android
	case "iOS":
		return iOS
	case "Mobile":
		return Mobile
	case "Desktop":
		return Desktop
	case "Tablet":
		return Tablet
	case "Others":
		return Others
	case "Bot":
		return Bot
	case "User1":
		return User1
	}
	return Unknown
}

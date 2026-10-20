//go:build desktop && darwin

package desktop

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Foundation
#import <Foundation/Foundation.h>

static int dengshell_system_dark(void) {
	@autoreleasepool {
		NSDictionary *global = [[NSUserDefaults standardUserDefaults] persistentDomainForName:NSGlobalDomain];
		return [[global objectForKey:@"AppleInterfaceStyle"] isEqualToString:@"Dark"];
	}
}
*/
import "C"

func platformSystemTheme() string {
	if C.dengshell_system_dark() != 0 {
		return "dark"
	}
	return "light"
}

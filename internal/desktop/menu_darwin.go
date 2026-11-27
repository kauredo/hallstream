package desktop

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework Cocoa
#import <Cocoa/Cocoa.h>

static NSString *str(const char *s) { return [NSString stringWithUTF8String:s]; }

static NSMenuItem *item(const char *title, SEL action, NSString *key, NSEventModifierFlags mods) {
	NSMenuItem *it = [[NSMenuItem alloc] initWithTitle:str(title) action:action keyEquivalent:key];
	it.keyEquivalentModifierMask = mods;
	return it;
}

static NSMenu *submenu(NSMenu *bar, const char *title) {
	NSMenuItem *top = [[NSMenuItem alloc] initWithTitle:str(title) action:nil keyEquivalent:@""];
	NSMenu *m = [[NSMenu alloc] initWithTitle:str(title)];
	top.submenu = m;
	[bar addItem:top];
	return m;
}

#define TITLES 23

// The order of t is the order of menuTitles in Go.
static void buildMenus(char **t) {
	char **c = malloc(TITLES * sizeof(char *));
	for (int i = 0; i < TITLES; i++) c[i] = strdup(t[i]);
	dispatch_async(dispatch_get_main_queue(), ^{
		NSEventModifierFlags cmd = NSEventModifierFlagCommand;
		NSMenu *bar = [[NSMenu alloc] init];

		NSMenu *app = submenu(bar, "Hallstream");
		[app addItem:item(c[0], @selector(orderFrontStandardAboutPanel:), @"", 0)];
		[app addItem:[NSMenuItem separatorItem]];
		[app addItem:item(c[1], @selector(hide:), @"h", cmd)];
		[app addItem:item(c[2], @selector(hideOtherApplications:), @"h", cmd | NSEventModifierFlagOption)];
		[app addItem:item(c[3], @selector(unhideAllApplications:), @"", 0)];
		[app addItem:[NSMenuItem separatorItem]];
		// terminate: reaches applicationShouldTerminate:, which runs Sair.
		[app addItem:item(c[4], @selector(terminate:), @"q", cmd)];

		NSMenu *edit = submenu(bar, c[5]);
		[edit addItem:item(c[6], @selector(undo:), @"z", cmd)];
		[edit addItem:item(c[7], @selector(redo:), @"z", cmd | NSEventModifierFlagShift)];
		[edit addItem:[NSMenuItem separatorItem]];
		[edit addItem:item(c[8], @selector(cut:), @"x", cmd)];
		[edit addItem:item(c[9], @selector(copy:), @"c", cmd)];
		[edit addItem:item(c[10], @selector(paste:), @"v", cmd)];
		[edit addItem:item(c[11], @selector(selectAll:), @"a", cmd)];

		// The hs… actions belong to the panel window's delegate, so they work
		// while the window is key and grey out while it is hidden.
		NSMenu *view = submenu(bar, c[15]);
		[view addItem:item(c[16], NSSelectorFromString(@"hsReload:"), @"r", cmd)];
		[view addItem:[NSMenuItem separatorItem]];
		[view addItem:item(c[17], NSSelectorFromString(@"hsZoomIn:"), @"+", cmd)];
		[view addItem:item(c[18], NSSelectorFromString(@"hsZoomOut:"), @"-", cmd)];
		[view addItem:item(c[19], NSSelectorFromString(@"hsActualSize:"), @"0", cmd)];
		[view addItem:[NSMenuItem separatorItem]];
		[view addItem:item(c[20], NSSelectorFromString(@"hsOpenInBrowser:"), @"", 0)];

		NSMenu *win = submenu(bar, c[12]);
		[win addItem:item(c[13], @selector(performMiniaturize:), @"m", cmd)];
		[win addItem:item(c[14], @selector(performClose:), @"w", cmd)];
		NSApp.windowsMenu = win;

		NSMenu *help = submenu(bar, c[21]);
		[help addItem:item(c[22], NSSelectorFromString(@"hsHelp:"), @"?", cmd)];
		NSApp.helpMenu = help;

		NSApp.mainMenu = bar;
		for (int i = 0; i < TITLES; i++) free(c[i]);
		free(c);
	});
}
*/
import "C"

import (
	"unsafe"

	"github.com/kauredo/hallstream/internal/i18n"
)

// menuTitles are the menu bar's words, in the order buildMenus reads them.
func menuTitles() []string {
	return []string{
		i18n.T("app.menu.acerca"), i18n.T("app.menu.ocultar"), i18n.T("app.menu.ocultar_outras"), i18n.T("app.menu.mostrar_todas"), i18n.T("app.menu.sair"),
		i18n.T("app.menu.editar"), i18n.T("app.menu.desfazer"), i18n.T("app.menu.refazer"), i18n.T("app.menu.cortar"), i18n.T("app.menu.copiar"), i18n.T("app.menu.colar"), i18n.T("app.menu.selecionar_tudo"),
		i18n.T("app.menu.janela"), i18n.T("app.menu.minimizar"), i18n.T("app.menu.fechar"),
		i18n.T("app.menu.ver"), i18n.T("app.menu.recarregar"), i18n.T("app.menu.aumentar"), i18n.T("app.menu.diminuir"), i18n.T("app.menu.tamanho_real"), i18n.T("app.menu.abrir_browser"),
		i18n.T("app.menu.ajuda"), i18n.T("app.menu.ajuda_hallstream"),
	}
}

// installMenus gives the app its menu bar once AppKit runs. Without one,
// macOS shows only the app's name and ⌘Q has no item to trigger.
func installMenus() {
	titles := menuTitles()
	if len(titles) != C.TITLES {
		panic("menuTitles and buildMenus disagree")
	}
	c := make([]*C.char, len(titles))
	for i, s := range titles {
		c[i] = C.CString(s)
		defer C.free(unsafe.Pointer(c[i]))
	}
	C.buildMenus(&c[0])
}

// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { globalStore, WOS } from "@/app/store/global";
import { RpcApi } from "@/app/store/wshclientapi";
import { TabRpcClient } from "@/app/store/wshrpcutil";
import { WebView, WebViewModel } from "@/app/view/webview/webview";
import { atom } from "jotai";

const docsiteUrl = `data:text/html,${encodeURIComponent(
    `<html><head><meta charset="utf-8"><style>body{background:#0b0b0b;color:#e5e5e5;font-family:Inter,system-ui,sans-serif;padding:40px}h1{font-size:22px;margin-bottom:8px}p{color:#a3a3a3;line-height:1.6}code{background:#1a1a1a;padding:2px 6px;border-radius:4px}</style></head><body><h1>Quasar Help</h1><p>Common shortcuts:</p><ul><li><code>Cmd/Ctrl + t</code> — new tab</li><li><code>Cmd/Ctrl + n</code> — new terminal block</li><li><code>Cmd/Ctrl + d</code> — split right</li><li><code>Cmd/Ctrl + Shift + d</code> — split down</li><li><code>Ctrl + Shift + arrows</code> — navigate blocks</li><li><code>Alt/Cmd + 1-9</code> — switch tab</li></ul><p>Open the Accounts view from the launcher to manage your AI CLI accounts.</p></body></html>`
)};`;

class HelpViewModel extends WebViewModel {
    get viewComponent(): ViewComponent {
        return HelpView;
    }

    constructor(initOpts: ViewModelInitType) {
        super(initOpts);
        this.viewText = atom((get) => {
            // force a dependency on meta.url so we re-render the buttons when the url changes
            void (get(this.blockAtom)?.meta?.url || get(this.homepageUrl));
            return [
                {
                    elemtype: "iconbutton",
                    icon: "chevron-left",
                    click: this.handleBack.bind(this),
                    disabled: this.shouldDisableBackButton(),
                },
                {
                    elemtype: "iconbutton",
                    icon: "chevron-right",
                    click: this.handleForward.bind(this),
                    disabled: this.shouldDisableForwardButton(),
                },
                {
                    elemtype: "iconbutton",
                    icon: "house",
                    click: this.handleHome.bind(this),
                    disabled: this.shouldDisableHomeButton(),
                },
            ];
        });
        this.homepageUrl = atom(docsiteUrl);
        this.viewType = "help";
        this.viewIcon = atom("circle-question");
        this.viewName = atom("Help");
    }

    setZoomFactor(factor: number | null) {
        // null is ok (will reset to default)
        if (factor != null && factor < 0.1) {
            factor = 0.1;
        }
        if (factor != null && factor > 5) {
            factor = 5;
        }
        const domReady = globalStore.get(this.domReady);
        if (!domReady) {
            return;
        }
        this.webviewRef.current?.setZoomFactor(factor || 1);
        RpcApi.SetMetaCommand(TabRpcClient, {
            oref: WOS.makeORef("block", this.blockId),
            meta: { "web:zoom": factor }, // allow null so we can remove the zoom factor here
        });
    }

    getSettingsMenuItems(): ContextMenuItem[] {
        const zoomSubMenu: ContextMenuItem[] = [];
        let curZoom = 1;
        if (globalStore.get(this.domReady)) {
            curZoom = this.webviewRef.current?.getZoomFactor() || 1;
        }
        // eslint-disable-next-line @typescript-eslint/no-this-alias
        const model = this; // for the closure to work (this is getting unset)
        function makeZoomFactorMenuItem(label: string, factor: number): ContextMenuItem {
            return {
                label: label,
                type: "checkbox",
                click: () => {
                    model.setZoomFactor(factor);
                },
                checked: curZoom == factor,
            };
        }
        zoomSubMenu.push({
            label: "Reset",
            click: () => {
                model.setZoomFactor(null);
            },
        });
        zoomSubMenu.push(makeZoomFactorMenuItem("25%", 0.25));
        zoomSubMenu.push(makeZoomFactorMenuItem("50%", 0.5));
        zoomSubMenu.push(makeZoomFactorMenuItem("70%", 0.7));
        zoomSubMenu.push(makeZoomFactorMenuItem("80%", 0.8));
        zoomSubMenu.push(makeZoomFactorMenuItem("90%", 0.9));
        zoomSubMenu.push(makeZoomFactorMenuItem("100%", 1));
        zoomSubMenu.push(makeZoomFactorMenuItem("110%", 1.1));
        zoomSubMenu.push(makeZoomFactorMenuItem("120%", 1.2));
        zoomSubMenu.push(makeZoomFactorMenuItem("130%", 1.3));
        zoomSubMenu.push(makeZoomFactorMenuItem("150%", 1.5));
        zoomSubMenu.push(makeZoomFactorMenuItem("175%", 1.75));
        zoomSubMenu.push(makeZoomFactorMenuItem("200%", 2));

        return [
            {
                label: this.webviewRef.current?.isDevToolsOpened() ? "Close DevTools" : "Open DevTools",
                click: async () => {
                    if (this.webviewRef.current) {
                        if (this.webviewRef.current.isDevToolsOpened()) {
                            this.webviewRef.current.closeDevTools();
                        } else {
                            this.webviewRef.current.openDevTools();
                        }
                    }
                },
            },
            {
                label: "Set Zoom Factor",
                submenu: zoomSubMenu,
            },
        ];
    }
}

function HelpView(props: ViewComponentProps<HelpViewModel>) {
    return (
        <div className="w-full h-full">
            <WebView {...props} />
        </div>
    );
}

export { HelpViewModel };

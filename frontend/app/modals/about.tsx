// Copyright 2026, Quasar
// SPDX-License-Identifier: Apache-2.0

import Logo from "@/app/asset/logo.svg";
import { atoms } from "@/app/store/global";
import { modalsModel } from "@/app/store/modalmodel";
import { isDev } from "@/util/isdev";
import { useAtomValue } from "jotai";
import { Modal } from "./modal";

interface AboutModalVProps {
    versionString: string;
    onClose: () => void;
}

const AboutModalV = ({ versionString, onClose }: AboutModalVProps) => {
    return (
        <Modal className="pt-[34px] pb-[34px] overflow-hidden w-[380px]" onClose={onClose}>
            <div className="flex flex-col gap-[22px] w-full relative z-10">
                <div className="flex flex-col items-center justify-center gap-4 self-stretch w-full text-center">
                    <div className="w-24 h-24 text-foreground [&_svg]:w-full [&_svg]:h-full">
                        <Logo />
                    </div>
                    <div className="text-[25px] font-medium">Quasar</div>
                    <div className="leading-5 text-secondary">Terminal with integrated AI account management</div>
                </div>
                <div className="items-center gap-4 self-stretch w-full text-center text-secondary">
                    Version {versionString}
                </div>
            </div>
        </Modal>
    );
};

AboutModalV.displayName = "AboutModalV";

const AboutModal = () => {
    const fullConfig = useAtomValue(atoms.fullConfigAtom);
    const versionString = `${fullConfig?.version ?? ""}${isDev() ? " (dev)" : ""}`;

    return <AboutModalV versionString={versionString} onClose={() => modalsModel.popModal()} />;
};

AboutModal.displayName = "AboutModal";

export { AboutModal, AboutModalV };

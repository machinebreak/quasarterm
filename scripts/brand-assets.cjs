// Copyright 2026, Quasar
// SPDX-License-Identifier: Apache-2.0

const fs = require("fs");
const path = require("path");
const sharp = require("sharp");
const { Potrace } = require("potrace");

const sourceDir = process.env.QUASAR_SOURCE_DIR || path.join(require("os").homedir(), "Downloads", "quasar");
const repoRoot = path.resolve(__dirname, "..");
const workDir = path.join(repoRoot, ".brand-work");
const outAssets = path.join(repoRoot, "assets");
const outPublic = path.join(repoRoot, "public", "logos");
const outAppAsset = path.join(repoRoot, "frontend", "app", "asset");
const outBuild = path.join(repoRoot, "build");

fs.mkdirSync(workDir, { recursive: true });
fs.mkdirSync(outAssets, { recursive: true });
fs.mkdirSync(outPublic, { recursive: true });
fs.mkdirSync(outAppAsset, { recursive: true });
fs.mkdirSync(path.join(outBuild, "icons"), { recursive: true });

async function makeTraceSource(srcFile, destFile, opts) {
    let img = sharp(path.join(sourceDir, srcFile)).ensureAlpha();
    if (opts.invert) {
        img = img.flatten({ background: "#000000" }).negate({ alpha: false });
    } else {
        img = img.flatten({ background: "#ffffff" });
    }
    await img.grayscale().png().toFile(destFile);
}

function traceToSvg(inputFile, outputFile, fill) {
    return new Promise((resolve, reject) => {
        const tracer = new Potrace({
            threshold: 160,
            turdSize: 4,
            optCurve: true,
            optTolerance: 0.2,
            alphaMax: 1,
            blackOnWhite: true,
        });
        tracer.loadImage(inputFile, function (err) {
            if (err) {
                return reject(err);
            }
            let svg = tracer.getSVG({ color: fill, background: "transparent" });
            svg = svg.replace(/<\?xml.*\?>\s*/, "");
            fs.writeFileSync(outputFile, svg);
            resolve();
        });
    });
}

async function makeRoundIcon(logoFile, destPng) {
    const size = 1024;
    const logoSize = 680;
    const logo = await sharp(path.join(sourceDir, logoFile))
        .resize(logoSize, logoSize, { fit: "contain", background: { r: 0, g: 0, b: 0, alpha: 0 } })
        .png()
        .toBuffer();
    const radius = Math.round(size * 0.22);
    const background = Buffer.from(
        `<svg width="${size}" height="${size}"><rect x="0" y="0" width="${size}" height="${size}" rx="${radius}" ry="${radius}" fill="#0b0b0b"/></svg>`
    );
    await sharp(background)
        .composite([{ input: logo, gravity: "center" }])
        .png()
        .toFile(destPng);
    return destPng;
}

async function makeTransparentWhiteLogo() {
    const src = sharp(path.join(sourceDir, "logowhite.png")).ensureAlpha();
    const { data, info } = await src.raw().toBuffer({ resolveWithObject: true });
    const out = Buffer.from(data);
    for (let i = 0; i < out.length; i += 4) {
        const r = out[i];
        const g = out[i + 1];
        const b = out[i + 2];
        const luminance = (r + g + b) / 3;
        if (luminance < 40) {
            out[i + 3] = 0;
        } else if (luminance < 90) {
            out[i + 3] = Math.round(((luminance - 40) / 50) * 255);
        }
    }
    const dest = path.join(workDir, "logowhite-transparent.png");
    await sharp(out, { raw: { width: info.width, height: info.height, channels: 4 } }).png().toFile(dest);
    return dest;
}

async function main() {
    const whiteTransparent = await makeTransparentWhiteLogo();
    fs.copyFileSync(whiteTransparent, path.join(outAssets, "quasar-logo-white.png"));
    fs.copyFileSync(path.join(sourceDir, "imagotipowhite.png"), path.join(outAssets, "quasar-imagotipo-white.png"));
    fs.copyFileSync(path.join(sourceDir, "imagotipoblack.png"), path.join(outAssets, "quasar-imagotipo-black.png"));
    fs.copyFileSync(path.join(sourceDir, "logodark.png"), path.join(outAssets, "quasar-logo-black.png"));

    const traceBlack = path.join(workDir, "logodark-trace.png");
    const traceWhite = path.join(workDir, "logowhite-trace.png");
    const traceBlackWide = path.join(workDir, "imagotipoblack-trace.png");
    const traceWhiteWide = path.join(workDir, "imagotipowhite-trace.png");

    await makeTraceSource("logodark.png", traceBlack, { invert: false });
    await makeTraceSource("logowhite.png", traceWhite, { invert: true });
    await makeTraceSource("imagotipoblack.png", traceBlackWide, { invert: false });
    await makeTraceSource("imagotipowhite.png", traceWhiteWide, { invert: true });

    await traceToSvg(traceBlack, path.join(outAssets, "quasar-logo.svg"), "#0b0b0b");
    await traceToSvg(traceWhite, path.join(outAssets, "quasar-logo-white.svg"), "#ffffff");
    await traceToSvg(traceBlackWide, path.join(outAssets, "quasar-imagotipo.svg"), "#0b0b0b");
    await traceToSvg(traceWhiteWide, path.join(outAssets, "quasar-imagotipo-white.svg"), "#ffffff");
    fs.copyFileSync(path.join(outAssets, "quasar-logo.svg"), path.join(outAppAsset, "quasar-logo.svg"));

    const iconPng = await makeRoundIcon("logowhite.png", path.join(outBuild, "icon.png"));
    for (const size of [16, 32, 48, 64, 128, 256, 512]) {
        await sharp(iconPng).resize(size, size).png().toFile(path.join(outBuild, "icons", `${size}x${size}.png`));
    }

    await sharp(whiteTransparent).resize(256, 256, { fit: "contain" }).png().toFile(path.join(outPublic, "quasar-logo-256.png"));
    await sharp(whiteTransparent).resize(512, 512, { fit: "contain" }).png().toFile(path.join(outPublic, "quasar-logo.png"));
    await sharp(path.join(sourceDir, "logodark.png")).resize(512, 512, { fit: "contain" }).png().toFile(path.join(outPublic, "quasar-logo-dark.png"));
    await sharp(whiteTransparent).resize(256, 256, { fit: "contain" }).png().toFile(path.join(outPublic, "quasar-logo-white.png"));

    for (const oldFile of ["icon.icns", "icon.ico"]) {
        const full = path.join(outBuild, oldFile);
        if (fs.existsSync(full)) {
            fs.rmSync(full);
        }
    }

    console.log("brand assets generated");
}

main().catch((err) => {
    console.error(err);
    process.exit(1);
});

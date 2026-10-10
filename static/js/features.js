/* SPDX-License-Identifier: MIT */

let panelFacade = null;
const features = new Map();

export function registerPanelFacade(facade) {
    if (panelFacade) throw new Error('panel facade is already registered');
    panelFacade = Object.freeze(Object.assign({}, facade));
}

export function getPanelFacade() {
    return panelFacade;
}

export function registerFeature(name, feature) {
    features.set(name, Object.freeze(Object.assign({}, feature)));
}

export function getFeature(name) {
    return features.get(name) || null;
}

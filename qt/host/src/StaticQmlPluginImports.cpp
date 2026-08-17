#include <QtPlugin>
#include <QResource>

static void f4InitializeStaticQtQmlResources()
{
    // Qt builds the Basic style's compiled QML into its backing archive, but
    // places the resource initializer in a separate object.  The native Qt
    // CMake package links that object automatically; Conan's component facade
    // does not expose it.  Referencing the resource explicitly pulls the QML
    // payload out of the static archive on both ELF and PE/COFF linkers.
    Q_INIT_RESOURCE(qtquickcontrols2basicstyle);
}

namespace
{
const bool f4StaticQtQmlResourcesInitialized = [] {
    f4InitializeStaticQtQmlResources();
    return true;
}();
}

Q_IMPORT_PLUGIN(QtQmlPlugin)
Q_IMPORT_PLUGIN(QtQmlModelsPlugin)
Q_IMPORT_PLUGIN(QtQmlWorkerScriptPlugin)
Q_IMPORT_PLUGIN(QtQuick2Plugin)
Q_IMPORT_PLUGIN(QtQuick_WindowPlugin)
Q_IMPORT_PLUGIN(QtQuickEffectsPlugin)
Q_IMPORT_PLUGIN(QmlShapesPlugin)
Q_IMPORT_PLUGIN(QtQuickLayoutsPlugin)
Q_IMPORT_PLUGIN(QtQuickTemplates2Plugin)
Q_IMPORT_PLUGIN(QtQuickControls2ImplPlugin)
Q_IMPORT_PLUGIN(QtQuickControls2BasicStyleImplPlugin)
Q_IMPORT_PLUGIN(QtQuickControls2BasicStylePlugin)
Q_IMPORT_PLUGIN(QtQuickControls2Plugin)

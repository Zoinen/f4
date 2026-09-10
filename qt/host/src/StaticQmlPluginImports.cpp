#include <QtPlugin>

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

// Conan's static Qt targets do not retain the resource initializer objects.
// Plugin registration alone exposes C++ types but loses qmldir dependencies,
// so imports can appear to work only while the build machine's Qt is installed.
// Keep the metadata for every module in our explicit static plugin set.
static void initializeStaticQmlResources()
{
    Q_INIT_RESOURCE(qmake_QML);
    Q_INIT_RESOURCE(qmake_QtQml);
    Q_INIT_RESOURCE(qmake_QtQml_Models);
    Q_INIT_RESOURCE(qmake_QtQml_WorkerScript);
    Q_INIT_RESOURCE(qmake_QtQuick);
    Q_INIT_RESOURCE(qmake_QtQuick_Window);
    Q_INIT_RESOURCE(qmake_QtQuick_Effects);
    Q_INIT_RESOURCE(qmake_QtQuick_Shapes);
    Q_INIT_RESOURCE(qmake_QtQuick_Layouts);
    Q_INIT_RESOURCE(qmake_QtQuick_Templates);
    Q_INIT_RESOURCE(qmake_QtQuick_Controls_impl);
    Q_INIT_RESOURCE(qmake_QtQuick_Controls_Basic_impl);
    Q_INIT_RESOURCE(qmake_QtQuick_Controls_Basic);
    Q_INIT_RESOURCE(qmake_QtQuick_Controls);
}
Q_CONSTRUCTOR_FUNCTION(initializeStaticQmlResources)

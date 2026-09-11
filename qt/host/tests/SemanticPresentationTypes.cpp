#include "ScenePixelAlignment.h"
#include "SemanticChildrenModel.h"
#include "SemanticOverlayModel.h"

#include <QCoreApplication>
#include <QtQml>

// Fixture executables load the QML sources directly and supply a fake grid.
// The production executable registers these same declarative types through
// qt6_add_qml_module, together with the real VtuiGridItem.
namespace {
void registerSemanticPresentationTypes()
{
    qmlRegisterTypesAndRevisions<SemanticChildrenModel, SemanticOverlayModel, ScenePixelAlignment>("F4QtHost", 1);
}
}
Q_COREAPP_STARTUP_FUNCTION(registerSemanticPresentationTypes)

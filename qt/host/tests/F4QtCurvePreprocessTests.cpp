#include <QQuickWindow>
#include <QQuickItem>
#include <QSGDynamicTexture>
#include <QSGTextureProvider>
#include <QtTest>
#define private public
#include <QtQuick/private/qsgrenderer_p.h>
#undef private
#include <QtQuick/private/qquickwindow_p.h>
#include <QtQuick/private/qsgcurvefillnode_p.h>
#include <QtQuick/private/qsgcurvestrokenode_p.h>
#include <atomic>
class CountingTexture final:public QSGDynamicTexture {
public:
 explicit CountingTexture(std::atomic_int *calls):calls(calls){}
 qint64 comparisonKey()const override{return qint64(quintptr(this));}
 QSize textureSize()const override{return {1,1};}
 bool hasAlphaChannel()const override{return false;}
 bool hasMipmaps()const override{return false;}
 bool updateTexture()override{++*calls;return false;}
 std::atomic_int *calls;
};
class CountingProvider final:public QSGTextureProvider {
public:
 CountingProvider(QObject *parent,std::atomic_int *calls){setParent(parent);value=new CountingTexture(calls);value->setParent(this);}
 QSGTexture *texture()const override{return value;}
 CountingTexture *value;
};
class CustomFillNode final:public QSGCurveFillNode {
public:
 explicit CustomFillNode(std::atomic_int *calls):calls(calls){}
 void preprocess()override{++*calls;QSGCurveFillNode::preprocess();}
 std::atomic_int *calls;
};
class CurveFixture final:public QQuickItem {
public:
 bool derived=false;int mode=0;QSGCurveFillNode *node=nullptr;CountingProvider *provider=nullptr;std::atomic_int calls=0;
 explicit CurveFixture(QQuickItem *p):QQuickItem(p){setFlag(ItemHasContents);setObjectName("noopPreprocessFixture");setPosition({8,8});setSize({32,32});}
 void setMode(int value){mode=value;update();}
 QSGNode *updatePaintNode(QSGNode *old,UpdatePaintNodeData*)override{
  if(!old)node=derived ? new CustomFillNode(&calls) : new QSGCurveFillNode;
  if(mode==1){if(!provider)provider=new CountingProvider(node,&calls);node->setFillTextureProvider(provider);}
  else if(mode==2){delete provider;provider=nullptr;}
  else node->setFillTextureProvider(nullptr);
  return node;
 }
};
class F4QtCurvePreprocessTests:public QObject {
 Q_OBJECT
 void checkPosition(QQuickWindow &window,CurveFixture *item){
  QCOMPARE(window.devicePixelRatio(),1.75);auto origin=item->mapToScene({});
  QVERIFY(qAbs(origin.x()*1.75-qRound(origin.x()*1.75))<.001);QVERIFY(qAbs(origin.y()*1.75-qRound(origin.y()*1.75))<.001);
  QCOMPARE(item->mapToScene({1,0})-origin,QPointF(1,0));QCOMPARE(item->mapToScene({0,1})-origin,QPointF(0,1));
 }
private slots:
 void untexturedStrokeIsNotRegistered(){
  class StrokeFixture final : public QQuickItem {
  public:
   QSGNode *node=nullptr;
   explicit StrokeFixture(QQuickItem *parent):QQuickItem(parent){
    setFlag(ItemHasContents);setObjectName("noopStrokeFixture");setPosition({8,8});setSize({32,32});
   }
   QSGNode *updatePaintNode(QSGNode *old,UpdatePaintNodeData*)override{
    if(!old){
     auto *stroke=new QSGCurveStrokeNode;
     stroke->cookGeometry();
     node=stroke;
    }
    return node;
   }
  };
  QQuickWindow window;window.resize(80,64);window.setColor(Qt::blue);
  auto *item=new StrokeFixture(window.contentItem());std::atomic_bool registered=true;
  auto connection=connect(&window,&QQuickWindow::beforeRendering,&window,[&]{
   auto *renderer=QQuickWindowPrivate::get(&window)->renderer;
   if(renderer&&item->node)registered=renderer->m_nodes_to_preprocess.contains(item->node);
  },Qt::DirectConnection);
  auto cleanup=qScopeGuard([&]{disconnect(connection);});
  window.show();QVERIFY(QTest::qWaitForWindowExposed(&window));auto image=window.grabWindow();
  QVERIFY2(!registered.load(),"Curve stroke still schedules an empty preprocess callback");
  QCOMPARE(window.devicePixelRatio(),1.75);QCOMPARE(item->mapToScene({}),QPointF(8,8));
  QCOMPARE(item->mapToScene({1,0}),QPointF(9,8));QCOMPARE(item->mapToScene({0,1}),QPointF(8,9));
  QCOMPARE(image.pixelColor(20,20),QColor(Qt::blue));
 }
 void untexturedFillIsNotRegistered(){
  QQuickWindow window;window.resize(80,64);window.setColor(Qt::blue);auto *item=new CurveFixture(window.contentItem());
  std::atomic_bool registered=true;
  auto connection=connect(&window,&QQuickWindow::beforeRendering,&window,[&]{auto *r=QQuickWindowPrivate::get(&window)->renderer;if(r&&item->node)registered=r->m_nodes_to_preprocess.contains(item->node);},Qt::DirectConnection);
  window.show();QVERIFY(QTest::qWaitForWindowExposed(&window));auto image=window.grabWindow();disconnect(connection);
  QVERIFY2(!registered.load(),"Untextured curve fill still schedules a no-op preprocess callback");checkPosition(window,item);QCOMPARE(image.pixelColor(20,20),QColor(Qt::blue));
 }
 void customDerivedCallbackIsPreserved(){
  QQuickWindow window;window.resize(80,64);auto *item=new CurveFixture(window.contentItem());item->derived=true;
  std::atomic_bool registered=false;
  auto connection=connect(&window,&QQuickWindow::beforeRendering,&window,[&]{auto *r=QQuickWindowPrivate::get(&window)->renderer;if(r&&item->node)registered=r->m_nodes_to_preprocess.contains(item->node);},Qt::DirectConnection);
  window.show();QVERIFY(QTest::qWaitForWindowExposed(&window));window.grabWindow();disconnect(connection);
  QVERIFY(registered.load());QVERIFY(item->calls.load()>0);checkPosition(window,item);
 }
 void textureProviderTransitionsPreserveUpdates(){
  QQuickWindow window;window.resize(80,64);window.setColor(Qt::blue);auto *item=new CurveFixture(window.contentItem());item->setMode(1);
  std::atomic_bool registered=false;
  auto connection=connect(&window,&QQuickWindow::beforeRendering,&window,[&]{auto *r=QQuickWindowPrivate::get(&window)->renderer;if(r&&item->node)registered=r->m_nodes_to_preprocess.contains(item->node);},Qt::DirectConnection);
  auto cleanup=qScopeGuard([&]{disconnect(connection);});
  window.show();QVERIFY(QTest::qWaitForWindowExposed(&window));window.grabWindow();QVERIFY(registered.load());QVERIFY(item->calls.load()>0);
  int before=item->calls;item->setMode(0);window.grabWindow();QVERIFY(!registered.load());QCOMPARE(item->calls.load(),before);
  item->setMode(1);window.grabWindow();QVERIFY(registered.load());QVERIFY(item->calls.load()>before);
  before=item->calls;item->setMode(2);auto image=window.grabWindow();QVERIFY(!registered.load());QCOMPARE(item->calls.load(),before);
  item->setMode(1);window.grabWindow();QVERIFY(registered.load());QVERIFY(item->calls.load()>before);checkPosition(window,item);
 }
};
QTEST_MAIN(F4QtCurvePreprocessTests)
#include "F4QtCurvePreprocessTests.moc"

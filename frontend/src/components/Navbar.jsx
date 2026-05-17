import { useEffect, useState } from 'react';
import { NavLink, useLocation, useNavigate } from 'react-router-dom';

const Navbar = () => {
  const navigate = useNavigate();
  const location = useLocation();
  const [menuOpen, setMenuOpen] = useState(false);
  const token = localStorage.getItem('token');
  const role = localStorage.getItem('role');
  const canUseMerchantConsole = role === 'merchant';
  const canBrowseMerchantWorkspace = role === 'merchant' || role === 'admin';

  useEffect(() => {
    setMenuOpen(false);
  }, [location.pathname]);

  const handleLogout = () => {
    localStorage.removeItem('token');
    localStorage.removeItem('user_id');
    localStorage.removeItem('role');
    navigate('/login');
  };

  return (
    <nav className="navbar">
      <div className="navbar-container">
        <NavLink to="/" className="navbar-brand">
          <span className="navbar-mark" aria-hidden="true">
            G
          </span>
          <span className="navbar-brand-copy">
            Go Commerce
            <small>Modern retail demo</small>
          </span>
        </NavLink>

        <button
          type="button"
          className="btn btn-secondary icon-button navbar-toggle"
          aria-label={menuOpen ? '收起导航菜单' : '展开导航菜单'}
          aria-expanded={menuOpen}
          onClick={() => setMenuOpen((value) => !value)}
        >
          {menuOpen ? '×' : '☰'}
        </button>

        <div className={`navbar-links ${menuOpen ? 'open' : ''}`.trim()}>
          <NavLink to="/" end className="navbar-link">
            首页
          </NavLink>
          <NavLink to="/products" className="navbar-link">
            商品
          </NavLink>
          {canBrowseMerchantWorkspace && (
            <NavLink to="/merchants" className="navbar-link">
              {role === 'admin' ? '商户管理' : '我的店铺'}
            </NavLink>
          )}
          {canUseMerchantConsole && (
            <NavLink to="/merchant" className="navbar-link">
              商家后台
            </NavLink>
          )}

          <div className="navbar-actions">
            {token ? (
              <>
                <NavLink to="/cart" className="navbar-link">
                  购物车
                </NavLink>
                <NavLink to="/orders" className="navbar-link">
                  订单
                </NavLink>
                <button type="button" className="btn btn-ghost" onClick={handleLogout}>
                  退出登录
                </button>
              </>
            ) : (
              <>
                <NavLink to="/login" className="navbar-link">
                  登录
                </NavLink>
                <NavLink to="/register" className="btn btn-primary btn-sm">
                  注册
                </NavLink>
              </>
            )}
          </div>
        </div>
      </div>
    </nav>
  );
};

export default Navbar;
